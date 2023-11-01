package upstream

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/upstream/bootstrap"
	"github.com/rnetx/cdns/upstream/pipeline"
	"github.com/rnetx/cdns/upstream/pool"
	"github.com/rnetx/cdns/utils"
	"github.com/rnetx/cdns/utils/network"
	"github.com/rnetx/cdns/utils/network/common"

	"github.com/miekg/dns"
)

type TLSUpstreamOptions struct {
	Address          string             `yaml:"address"`
	ConnectTimeout   utils.Duration     `yaml:"connect-timeout,omitempty"`
	IdleTimeout      utils.Duration     `yaml:"idle-timeout,omitempty"`
	EnablePipeline   bool               `yaml:"enable-pipeline,omitempty"`
	TLSOptions       TLSOptions         `yaml:",inline,omitempty"`
	BootstrapOptions *bootstrap.Options `yaml:"bootstrap,omitempty"`
	DialerOptions    network.Options    `yaml:",inline,omitempty"`
}

const TLSUpstreamType = "tls"

var (
	_ adapter.Upstream = (*TLSUpstream)(nil)
	_ adapter.Starter  = (*TLSUpstream)(nil)
	_ adapter.Closer   = (*TLSUpstream)(nil)
)

type TLSUpstream struct {
	ctx    context.Context
	tag    string
	core   adapter.Core
	logger log.Logger

	address   common.SocksAddr
	dialer    common.Dialer
	bootstrap *bootstrap.Bootstrap

	connectTimeout time.Duration
	idleTimeout    time.Duration

	tlsConfig *tls.Config

	enablePipeline bool

	tlsPipelinePool *pipeline.DNSPipelineConnPool
	tlsConnPool     *pool.Pool[*dns.Conn]

	reqTotal   atomic.Uint64
	reqSuccess atomic.Uint64
}

func NewTLSUpstream(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options TLSUpstreamOptions) (adapter.Upstream, error) {
	u := &TLSUpstream{
		ctx:    ctx,
		tag:    tag,
		core:   core,
		logger: logger,
	}
	socksAddr, err := common.NewSocksAddrFromStringWithDefaultPort(options.Address, 853)
	if err != nil {
		return nil, fmt.Errorf("create tls upstream failed: invalid address: %s, error: %s", options.Address, err)
	}
	u.address = *socksAddr
	dialer, err := network.NewDialer(options.DialerOptions)
	if err != nil {
		return nil, fmt.Errorf("create tls upstream failed: create dialer: %s", err)
	}
	u.dialer = dialer
	if options.BootstrapOptions != nil {
		b, err := bootstrap.NewBootstrap(ctx, core, *options.BootstrapOptions)
		if err != nil {
			return nil, fmt.Errorf("create tls upstream failed: create bootstrap: %s", err)
		}
		u.bootstrap = b
	}
	if u.address.IsDomain() && !network.IsSocks5Dialer(u.dialer) && u.bootstrap == nil {
		return nil, fmt.Errorf("create tls upstream failed: domain address requires socks5 dialer or bootstrap")
	}
	if options.ConnectTimeout > 0 {
		u.connectTimeout = time.Duration(options.ConnectTimeout)
	} else {
		u.connectTimeout = DefaultConnectTimeout
	}
	if options.IdleTimeout > 0 {
		u.idleTimeout = time.Duration(options.IdleTimeout)
	} else {
		u.idleTimeout = DefaultIdleTimeout
	}
	u.enablePipeline = options.EnablePipeline
	tlsConfig, err := newTLSConfig(options.TLSOptions)
	if err != nil {
		return nil, fmt.Errorf("create tls upstream failed: create tls config: %s", err)
	}
	tlsConfig.Time = core.GetTimeFunc()
	if tlsConfig.ServerName == "" {
		if u.address.IsDomain() {
			tlsConfig.ServerName = u.address.Domain()
		} else {
			tlsConfig.ServerName = u.address.IP().String()
		}
	}
	tlsConfig.NextProtos = []string{"dns"}
	u.tlsConfig = tlsConfig
	return u, nil
}

func (u *TLSUpstream) Tag() string {
	return u.tag
}

func (u *TLSUpstream) Type() string {
	return TLSUpstreamType
}

func (u *TLSUpstream) Dependencies() []string {
	if u.bootstrap != nil {
		return []string{u.bootstrap.UpstreamTag()}
	}
	return nil
}

func (u *TLSUpstream) Start() error {
	if u.bootstrap != nil {
		err := u.bootstrap.Start()
		if err != nil {
			return fmt.Errorf("start bootstrap failed: %s", err)
		}
	}
	if !u.enablePipeline {
		u.tlsConnPool = pool.NewPool(u.ctx, 0, u.idleTimeout, func(ctx context.Context) (*dns.Conn, error) {
			conn, err := u.newTLSConn(ctx)
			if err != nil {
				return nil, err
			}
			u.logger.Debug("new tls connection")
			return &dns.Conn{Conn: conn}, nil
		}, func(conn *dns.Conn) {
			conn.Close()
			u.logger.Debug("tls connection closed")
		})
	} else {
		u.tlsPipelinePool = pipeline.NewDNSPipelineConnPool(u.ctx, 0, DefaultUDPBufferSize, u.idleTimeout, func(ctx context.Context) (net.Conn, error) {
			conn, err := u.newTLSConn(ctx)
			if err != nil {
				return nil, err
			}
			u.logger.Debug("new tls pipeline connection")
			return conn, nil
		}, func() {
			u.logger.Debug("tls pipeline connection closed")
		})
	}
	return nil
}

func (u *TLSUpstream) Close() error {
	if u.bootstrap != nil {
		u.bootstrap.Close()
	}
	if !u.enablePipeline {
		u.tlsConnPool.Close()
	} else {
		u.tlsPipelinePool.Close()
	}
	return nil
}

func (u *TLSUpstream) newTCPConn(ctx context.Context) (net.Conn, error) {
	if u.address.IsDomain() {
		if u.bootstrap != nil {
			domain := u.address.Domain()
			ips, err := u.bootstrap.Lookup(ctx, domain)
			if err != nil {
				return nil, fmt.Errorf("lookup domain failed: %s, error: %s", domain, err)
			}
			conn, _, err := network.DialParallel(ctx, u.dialer, "tcp", ips, u.address.Port())
			return conn, err
		}
	}
	return u.dialer.DialContext(ctx, "tcp", u.address)
}

func (u *TLSUpstream) newTLSConn(ctx context.Context) (net.Conn, error) {
	conn, err := u.newTCPConn(ctx)
	if err != nil {
		return nil, err
	}
	tlsConn := tls.Client(conn, u.tlsConfig.Clone())
	err = tlsConn.HandshakeContext(ctx)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("tls handshake failed: %s", err)
	}
	return tlsConn, nil
}

func (u *TLSUpstream) exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	if !u.enablePipeline {
		conn, err := u.tlsConnPool.Get(ctx)
		if err != nil {
			return nil, fmt.Errorf("get tcp connection failed: %s", err)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			deadline = time.Now().Add(DefaultQueryTimeout)
		}
		err = conn.SetDeadline(deadline)
		if err != nil {
			return nil, fmt.Errorf("set tcp connection deadline failed: %s", err)
		}
		err = conn.WriteMsg(req)
		if err != nil {
			return nil, fmt.Errorf("send dns message failed: %s", err)
		}
		resp, err := conn.ReadMsg()
		if err != nil {
			return nil, fmt.Errorf("receive dns message failed: %s", err)
		}
		u.tlsConnPool.Put(ctx, conn)
		return resp, nil
	} else {
		return u.tlsPipelinePool.Exchange(ctx, req)
	}
}

func (u *TLSUpstream) Exchange(ctx context.Context, req *dns.Msg) (resp *dns.Msg, err error) {
	resp, err = Exchange(ctx, req, u.logger, u.exchange)
	u.reqTotal.Add(1)
	if err == nil {
		u.reqSuccess.Add(1)
	}
	return
}

func (u *TLSUpstream) StatisticalData() map[string]any {
	total := u.reqTotal.Load()
	success := u.reqSuccess.Load()
	return map[string]any{
		"total":   total,
		"success": success,
	}
}
