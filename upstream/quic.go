package upstream

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/upstream/bootstrap"
	"github.com/rnetx/cdns/upstream/network"
	"github.com/rnetx/cdns/upstream/network/common"
	"github.com/rnetx/cdns/utils"
)

type QUICUpstreamOptions struct {
	Address          string             `yaml:"address"`
	ConnectTimeout   utils.Duration     `yaml:"connect-timeout,omitempty"`
	IdleTimeout      utils.Duration     `yaml:"idle-timeout,omitempty"`
	TLSOptions       TLSOptions         `yaml:",inline,omitempty"`
	BootstrapOptions *bootstrap.Options `yaml:"bootstrap,omitempty"`
	DialerOptions    network.Options    `yaml:",inline,omitempty"`
}

const QUICUpstreamType = "quic"

var (
	_ adapter.Upstream = (*QUICUpstream)(nil)
	_ adapter.Starter  = (*QUICUpstream)(nil)
	_ adapter.Closer   = (*QUICUpstream)(nil)
)

type QUICUpstream struct {
	ctx    context.Context
	tag    string
	core   adapter.Core
	logger log.Logger

	address   common.SocksAddr
	dialer    common.Dialer
	bootstrap *bootstrap.Bootstrap

	connectTimeout time.Duration
	idleTimeout    time.Duration

	tlsConfig  *tls.Config
	quicConfig *quic.Config

	loopCtx            context.Context
	loopCancel         context.CancelFunc
	closeDone          chan struct{}
	isClosed           bool
	quicConnectionLock sync.Mutex
	quicConnection     *quicConnection

	reqTotal   atomic.Uint64
	reqSuccess atomic.Uint64
}

func NewQUICUpstream(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options QUICUpstreamOptions) (adapter.Upstream, error) {
	u := &QUICUpstream{
		ctx:    ctx,
		tag:    tag,
		core:   core,
		logger: logger,
	}
	socksAddr, err := common.NewSocksAddrFromStringWithDefaultPort(options.Address, 853)
	if err != nil {
		return nil, fmt.Errorf("create quic upstream failed: invalid address: %s, error: %s", options.Address, err)
	}
	u.address = *socksAddr
	dialer, err := network.NewDialer(options.DialerOptions)
	if err != nil {
		return nil, fmt.Errorf("create quic upstream failed: create dialer: %s", err)
	}
	u.dialer = dialer
	if options.BootstrapOptions != nil {
		b, err := bootstrap.NewBootstrap(ctx, core, *options.BootstrapOptions)
		if err != nil {
			return nil, fmt.Errorf("create quic upstream failed: create bootstrap: %s", err)
		}
		u.bootstrap = b
	}
	if u.address.IsDomain() && !network.IsSocks5Dialer(u.dialer) && u.bootstrap == nil {
		return nil, fmt.Errorf("create quic upstream failed: domain address requires socks5 dialer or bootstrap")
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
	tlsConfig, err := newTLSConfig(options.TLSOptions)
	if err != nil {
		return nil, fmt.Errorf("create quic upstream failed: create tls config: %s", err)
	}
	if tlsConfig.ServerName == "" {
		if u.address.IsDomain() {
			tlsConfig.ServerName = u.address.Domain()
		} else {
			tlsConfig.ServerName = u.address.IP().String()
		}
	}
	tlsConfig.NextProtos = []string{"doq"}
	tlsConfig.MinVersion = tls.VersionTLS13
	u.tlsConfig = tlsConfig
	u.quicConfig = &quic.Config{}
	return u, nil
}

func (u *QUICUpstream) Tag() string {
	return u.tag
}

func (u *QUICUpstream) Type() string {
	return QUICUpstreamType
}

func (u *QUICUpstream) Dependencies() []string {
	if u.bootstrap != nil {
		return []string{u.bootstrap.UpstreamTag()}
	}
	return nil
}

func (u *QUICUpstream) Start() error {
	if u.bootstrap != nil {
		err := u.bootstrap.Start()
		if err != nil {
			return fmt.Errorf("start bootstrap failed: %s", err)
		}
	}
	u.loopCtx, u.loopCancel = context.WithCancel(u.ctx)
	u.closeDone = make(chan struct{}, 1)
	go u.loopHandle()
	return nil
}

func (u *QUICUpstream) Close() error {
	if u.bootstrap != nil {
		u.bootstrap.Close()
	}
	if !u.isClosed {
		u.isClosed = true
		u.loopCancel()
		<-u.closeDone
		close(u.closeDone)
	}
	if u.quicConnection != nil {
		u.quicConnection.Close()
		u.quicConnection = nil
	}
	return nil
}

func (u *QUICUpstream) loopHandle() {
	defer func() {
		select {
		case u.closeDone <- struct{}{}:
		default:
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-u.loopCtx.Done():
			return
		case <-ticker.C:
			if u.quicConnection != nil {
				u.quicConnectionLock.Lock()
				if time.Since(u.quicConnection.LastUse()) > u.idleTimeout {
					u.quicConnection.Close()
					u.quicConnection = nil
				}
				u.quicConnectionLock.Unlock()
			}
		}
	}
}

func (u *QUICUpstream) newUDPPacketConn(ctx context.Context) (net.PacketConn, net.Addr, error) {
	if u.address.IsDomain() {
		if u.bootstrap != nil {
			domain := u.address.Domain()
			ips, err := u.bootstrap.Lookup(ctx, domain)
			if err != nil {
				return nil, nil, fmt.Errorf("lookup domain failed: %s, error: %s", domain, err)
			}
			conn, ip, err := network.ListenParallel(ctx, u.dialer, ips, u.address.Port())
			return conn, &net.UDPAddr{IP: ip.AsSlice(), Port: int(u.address.Port())}, err
		}
	}
	udpConn, err := u.dialer.ListenPacket(ctx, u.address)
	if err != nil {
		return nil, nil, err
	}
	return udpConn, u.address.UDPAddr(), nil
}

func (u *QUICUpstream) newQUICConnection(ctx context.Context) (*quicConnection, error) {
	udpConn, remoteAddr, err := u.newUDPPacketConn(ctx)
	if err != nil {
		return nil, err
	}
	newInner, err := quic.DialEarly(ctx, udpConn, remoteAddr, u.tlsConfig.Clone(), u.quicConfig.Clone())
	if err != nil {
		return nil, err
	}
	conn := newQUICConnection(newInner, func() {
		u.logger.Debug("quic connection closed")
	})
	u.logger.Debug("new quic connection")
	return conn, nil
}

func (u *QUICUpstream) getQUICConnection(ctx context.Context) (*quicConnection, error) {
	conn := u.quicConnection
	if conn != nil && !conn.ConnectionIsClosed() {
		return conn.Clone(), nil
	}
	u.quicConnectionLock.Lock()
	defer u.quicConnectionLock.Unlock()
	if u.quicConnection != nil && !u.quicConnection.ConnectionIsClosed() {
		return u.quicConnection.Clone(), nil
	}
	if u.quicConnection != nil {
		u.quicConnection.Close()
		u.quicConnection = nil
	}
	newConn, err := u.newQUICConnection(ctx)
	if err != nil {
		return nil, err
	}
	u.quicConnection = newConn
	return newConn.Clone(), nil
}

func (u *QUICUpstream) exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	raw, err := req.Pack()
	if err != nil {
		return nil, err
	}
	conn, err := u.getQUICConnection(ctx)
	if err != nil {
		return nil, fmt.Errorf("get quic connection failed: %s", err)
	}
	defer conn.Close()
	stream, err := conn.NewQUICStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("create quic stream failed: %s", err)
	}
	buffer := bytes.NewBuffer(make([]byte, 0, len(raw)+2))
	binary.Write(buffer, binary.BigEndian, uint16(len(raw)))
	buffer.Write(raw)
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(DefaultQueryTimeout)
	}
	err = stream.SetDeadline(deadline)
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("set quic stream deadline failed: %s", err)
	}
	_, err = stream.Write(buffer.Bytes())
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("write quic stream failed: %s", err)
	}
	stream.Close()
	buf, err := io.ReadAll(stream)
	if err != nil {
		return nil, fmt.Errorf("read quic stream failed: %s", err)
	}
	resp := &dns.Msg{}
	err = resp.Unpack(buf[2:])
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (u *QUICUpstream) Exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	newReq := new(dns.Msg)
	*newReq = *req
	newReq.Id = 0
	resp, err := Exchange(ctx, newReq, u.logger, u.exchange)
	u.reqTotal.Add(1)
	if err == nil {
		resp.Id = req.Id
		u.reqSuccess.Add(1)
	}
	return resp, err
}

func (u *QUICUpstream) StatisticalData() map[string]any {
	total := u.reqTotal.Load()
	success := u.reqSuccess.Load()
	return map[string]any{
		"total":   total,
		"success": success,
	}
}

type quicConnection struct {
	n         *atomic.Int64
	lastUse   time.Time
	inner     quic.Connection
	closeFunc func()
}

func newQUICConnection(inner quic.Connection, closeFunc func()) *quicConnection {
	c := &quicConnection{
		n:         &atomic.Int64{},
		lastUse:   time.Now(),
		inner:     inner,
		closeFunc: closeFunc,
	}
	c.n.Add(1)
	return c
}

func (c *quicConnection) flushLastUse() {
	c.lastUse = time.Now()
}

func (c *quicConnection) LastUse() time.Time {
	return c.lastUse
}

func (c *quicConnection) Clone() *quicConnection {
	c.n.Add(1)
	c.flushLastUse()
	return c
}

func (c *quicConnection) ConnectionIsClosed() bool {
	return utils.IsContextCancelled(c.inner.Context())
}

func (c *quicConnection) Close() {
	if c.n.Add(-1) == 0 {
		c.inner.CloseWithError(0, "")
		if c.closeFunc != nil {
			c.closeFunc()
		}
	}
}

func (c *quicConnection) NewQUICStream(ctx context.Context) (quic.Stream, error) {
	return c.inner.OpenStreamSync(ctx)
}
