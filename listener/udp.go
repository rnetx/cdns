package listener

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
)

type UDPListenerOptions struct {
	Listen        string `yaml:"listen"`
	MaxConnection int    `yaml:"max-connection,omitempty"`
}

const (
	UDPListenerType  = "udp"
	UDPMaxBufferSize = 4096
)

var (
	_ adapter.Listener = (*UDPListener)(nil)
	_ adapter.Starter  = (*UDPListener)(nil)
	_ adapter.Closer   = (*UDPListener)(nil)
)

type UDPListener struct {
	ctx    context.Context
	cancel context.CancelFunc
	tag    string
	core   adapter.Core
	logger log.Logger

	listen      string
	workflowTag string
	workflow    adapter.Workflow

	maxConnection int

	limiter *utils.Limiter
	udpConn net.PacketConn
}

func NewUDPListener(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options UDPListenerOptions, workflow string) (adapter.Listener, error) {
	ctx, cancel := context.WithCancel(ctx)
	l := &UDPListener{
		ctx:    ctx,
		cancel: cancel,
		tag:    tag,
		core:   core,
		logger: logger,
	}
	var err error
	l.listen, err = parseListen(options.Listen, 53)
	if err != nil {
		return nil, fmt.Errorf("create udp listener failed: %s", err)
	}
	if options.MaxConnection > 0 {
		l.maxConnection = options.MaxConnection
	} else {
		l.maxConnection = DefaultMaxConnection
	}
	if workflow == "" {
		return nil, fmt.Errorf("create udp listener failed: missing workflow")
	}
	l.workflowTag = workflow
	return l, nil
}

func (l *UDPListener) Tag() string {
	return l.tag
}

func (l *UDPListener) Type() string {
	return UDPListenerType
}

func (l *UDPListener) Start() error {
	w := l.core.GetWorkflow(l.workflowTag)
	if w == nil {
		return fmt.Errorf("create udp listener failed: workflow [%s] not found", l.workflowTag)
	}
	l.workflow = w
	l.limiter = utils.NewLimiter(l.maxConnection)
	udpAddr, err := net.ResolveUDPAddr("udp", l.listen)
	if err != nil {
		return fmt.Errorf("resolve udp address failed: %s", err)
	}
	l.udpConn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listen udp failed: %s, error: %s", udpAddr.String(), err)
	}
	l.logger.Infof("udp listener: listen %s", l.listen)
	go l.loopHandle()
	return nil
}

func (l *UDPListener) Close() error {
	l.cancel()
	l.udpConn.Close()
	return nil
}

func (l *UDPListener) loopHandle() {
	for {
		if !l.limiter.Get(l.ctx) {
			l.limiter.PutBack()
			return
		}
		buffer := make([]byte, UDPMaxBufferSize)
		n, remoteAddr, err := l.udpConn.ReadFrom(buffer)
		if err != nil {
			l.limiter.PutBack()
			return
		}
		addr, err := netip.ParseAddrPort(remoteAddr.String())
		if err != nil {
			l.logger.Debugf("parse client address failed: %s", err)
			l.limiter.PutBack()
			continue
		}
		go l.serve(buffer[:n], addr)
	}
}

func (l *UDPListener) serve(buf []byte, addr netip.AddrPort) {
	defer l.limiter.PutBack()
	req := &dns.Msg{}
	err := req.Unpack(buf)
	if err != nil {
		l.logger.Debugf("unpack dns message failed: client address: %s, error: %s", addr.String(), err)
		return
	}
	resp := l.Handle(l.ctx, req, addr)
	if resp != nil {
		resp.Truncate(getUDPSize(req))
		raw, err := resp.Pack()
		if err != nil {
			l.logger.Debugf("pack dns message failed: client address: %s, error: %s", addr.String(), err)
			return
		}
		_, err = l.udpConn.WriteTo(raw, &net.UDPAddr{IP: addr.Addr().AsSlice(), Port: int(addr.Port())})
		if err != nil {
			l.logger.Debugf("write dns message failed: client address: %s, error: %s", addr.String(), err)
		}
	}
}

func (l *UDPListener) Handle(ctx context.Context, req *dns.Msg, clientAddr netip.AddrPort) *dns.Msg {
	return listenerHandle(ctx, l.listen, l.logger, l.workflow, req, clientAddr)
}

// from mosdns(https://github.com/IrineSistiana/mosdns), thank for @IrineSistiana
func getUDPSize(m *dns.Msg) int {
	var s uint16
	if opt := m.IsEdns0(); opt != nil {
		s = opt.UDPSize()
	}
	if s < dns.MinMsgSize {
		s = dns.MinMsgSize
	}
	return int(s)
}
