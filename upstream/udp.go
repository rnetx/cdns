package upstream

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/upstream/bootstrap"
	"github.com/rnetx/cdns/upstream/network"
	"github.com/rnetx/cdns/upstream/network/common"
	"github.com/rnetx/cdns/upstream/pipeline"
	"github.com/rnetx/cdns/upstream/pool"
	"github.com/rnetx/cdns/utils"
)

type UDPUpstreamOptions struct {
	Address          string             `yaml:"address"`
	ConnectTimeout   utils.Duration     `yaml:"connect-timeout,omitempty"`
	IdleTimeout      utils.Duration     `yaml:"idle-timeout,omitempty"`
	FallbackTCP      bool               `yaml:"fallback-tcp,omitempty"`
	EnablePipeline   bool               `yaml:"enable-pipeline,omitempty"`
	BootstrapOptions *bootstrap.Options `yaml:"bootstrap,omitempty"`
	DialerOptions    network.Options    `yaml:",inline,omitempty"`
}

const DefaultUDPBufferSize = 4096

const UDPUpstreamType = "udp"

var (
	_ adapter.Upstream = (*UDPUpstream)(nil)
	_ adapter.Starter  = (*UDPUpstream)(nil)
	_ adapter.Closer   = (*UDPUpstream)(nil)
)

type UDPUpstream struct {
	ctx    context.Context
	tag    string
	core   adapter.Core
	logger log.Logger

	address   common.SocksAddr
	dialer    common.Dialer
	bootstrap *bootstrap.Bootstrap

	connectTimeout time.Duration
	idleTimeout    time.Duration

	fallbackTCP    bool
	enablePipeline bool

	udpConnPool     *pool.Pool[*dns.Conn]
	tcpPipelinePool *pipeline.DNSPipelineConnPool
	tcpConnPool     *pool.Pool[*dns.Conn]

	reqTotal   atomic.Uint64
	reqSuccess atomic.Uint64
}

func NewUDPUpstream(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options UDPUpstreamOptions) (adapter.Upstream, error) {
	u := &UDPUpstream{
		ctx:    ctx,
		tag:    tag,
		core:   core,
		logger: logger,
	}
	socksAddr, err := common.NewSocksAddrFromStringWithDefaultPort(options.Address, 53)
	if err != nil {
		return nil, fmt.Errorf("create udp upstream failed: invalid address: %s, error: %s", options.Address, err)
	}
	u.address = *socksAddr
	dialer, err := network.NewDialer(options.DialerOptions)
	if err != nil {
		return nil, fmt.Errorf("create udp upstream failed: create dialer: %s", err)
	}
	u.dialer = dialer
	if options.BootstrapOptions != nil {
		b, err := bootstrap.NewBootstrap(ctx, core, *options.BootstrapOptions)
		if err != nil {
			return nil, fmt.Errorf("create udp upstream failed: create bootstrap: %s", err)
		}
		u.bootstrap = b
	}
	if u.address.IsDomain() && !network.IsSocks5Dialer(u.dialer) && u.bootstrap == nil {
		return nil, fmt.Errorf("create udp upstream failed: domain address requires socks5 dialer or bootstrap")
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
	u.fallbackTCP = options.FallbackTCP
	u.enablePipeline = options.EnablePipeline
	return u, nil
}

func (u *UDPUpstream) Tag() string {
	return u.tag
}

func (u *UDPUpstream) Type() string {
	return UDPUpstreamType
}

func (u *UDPUpstream) Dependencies() []string {
	if u.bootstrap != nil {
		return []string{u.bootstrap.UpstreamTag()}
	}
	return nil
}

func (u *UDPUpstream) Start() error {
	if u.bootstrap != nil {
		err := u.bootstrap.Start()
		if err != nil {
			return fmt.Errorf("start bootstrap failed: %s", err)
		}
	}
	u.udpConnPool = pool.NewPool(u.ctx, 0, u.idleTimeout, func(ctx context.Context) (*dns.Conn, error) {
		conn, err := u.newUDPConn(ctx)
		if err != nil {
			return nil, err
		}
		u.logger.Debug("new udp connection")
		return &dns.Conn{
			Conn:    conn,
			UDPSize: DefaultUDPBufferSize,
		}, nil
	}, func(conn *dns.Conn) {
		conn.Close()
		u.logger.Debug("udp connection closed")
	})
	if u.fallbackTCP {
		if !u.enablePipeline {
			u.tcpConnPool = pool.NewPool(u.ctx, 0, u.idleTimeout, func(ctx context.Context) (*dns.Conn, error) {
				conn, err := u.newTCPConn(ctx)
				if err != nil {
					return nil, err
				}
				u.logger.Debug("new tcp connection")
				return &dns.Conn{Conn: conn}, nil
			}, func(conn *dns.Conn) {
				conn.Close()
				u.logger.Debug("tcp connection closed")
			})
		} else {
			u.tcpPipelinePool = pipeline.NewDNSPipelineConnPool(u.ctx, 0, DefaultUDPBufferSize, u.idleTimeout, func(ctx context.Context) (net.Conn, error) {
				conn, err := u.newTCPConn(ctx)
				if err != nil {
					return nil, err
				}
				u.logger.Debug("new tcp pipeline connection")
				return conn, nil
			}, func() {
				u.logger.Debug("tcp pipeline connection closed")
			})
		}
	}
	return nil
}

func (u *UDPUpstream) Close() error {
	if u.bootstrap != nil {
		u.bootstrap.Close()
	}
	u.udpConnPool.Close()
	if u.fallbackTCP {
		if !u.enablePipeline {
			u.tcpConnPool.Close()
		} else {
			u.tcpPipelinePool.Close()
		}
	}
	return nil
}

func (u *UDPUpstream) newUDPConn(ctx context.Context) (net.Conn, error) {
	if u.address.IsDomain() {
		if u.bootstrap != nil {
			domain := u.address.Domain()
			ips, err := u.bootstrap.Lookup(ctx, domain)
			if err != nil {
				return nil, fmt.Errorf("lookup domain failed: %s, error: %s", domain, err)
			}
			conn, _, err := network.DialParallel(ctx, u.dialer, "udp", ips, u.address.Port())
			return conn, err
		}
	}
	return u.dialer.DialContext(ctx, "udp", u.address)
}

func (u *UDPUpstream) newTCPConn(ctx context.Context) (net.Conn, error) {
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

func (u *UDPUpstream) exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	raw, err := req.Pack()
	if err != nil {
		return nil, fmt.Errorf("pack request failed: %s", err)
	}
	if len(raw) > 512 {
		if !u.fallbackTCP {
			return nil, fmt.Errorf("request too large: %d", len(raw))
		}
		// TCP
		if !u.enablePipeline {
			conn, err := u.tcpConnPool.Get(ctx)
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
			u.tcpConnPool.Put(ctx, conn)
			return resp, nil
		} else {
			return u.tcpPipelinePool.Exchange(ctx, req)
		}
	} else {
		// UDP
		conn, err := u.udpConnPool.Get(ctx)
		if err != nil {
			return nil, fmt.Errorf("get udp connection failed: %s", err)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			deadline = time.Now().Add(DefaultQueryTimeout)
		}
		err = conn.SetDeadline(deadline)
		if err != nil {
			return nil, fmt.Errorf("set udp connection deadline failed: %s", err)
		}
		err = conn.WriteMsg(req)
		if err != nil {
			return nil, fmt.Errorf("send dns message failed: %s", err)
		}
		resp, err := conn.ReadMsg()
		if err != nil {
			return nil, fmt.Errorf("receive dns message failed: %s", err)
		}
		u.udpConnPool.Put(ctx, conn)
		return resp, nil
	}
}

func (u *UDPUpstream) Exchange(ctx context.Context, req *dns.Msg) (resp *dns.Msg, err error) {
	resp, err = Exchange(ctx, req, u.logger, u.exchange)
	u.reqTotal.Add(1)
	if err == nil {
		u.reqSuccess.Add(1)
	}
	return
}

func (u *UDPUpstream) StatisticalData() map[string]any {
	total := u.reqTotal.Load()
	success := u.reqSuccess.Load()
	return map[string]any{
		"total":   total,
		"success": success,
	}
}
