package listener

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
)

type TLSListenerOptions struct {
	Listen        string         `yaml:"listen"`
	IdleTimeout   utils.Duration `yaml:"idle-timeout,omitempty"`
	MaxConnection int            `yaml:"max-connection,omitempty"`
	TLSOptions    TLSOptions     `yaml:",inline,omitempty"`
}

const TLSListenerType = "tls"

var (
	_ adapter.Listener = (*TLSListener)(nil)
	_ adapter.Starter  = (*TLSListener)(nil)
	_ adapter.Closer   = (*TLSListener)(nil)
)

type TLSListener struct {
	ctx    context.Context
	cancel context.CancelFunc
	tag    string
	core   adapter.Core
	logger log.Logger

	listen      string
	workflowTag string
	workflow    adapter.Workflow

	idleTimeout   time.Duration
	maxConnection int
	tlsConfig     *tls.Config

	limiter     *utils.Limiter
	tlsListener net.Listener
}

func NewTLSListener(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options TLSListenerOptions, workflow string) (adapter.Listener, error) {
	ctx, cancel := context.WithCancel(ctx)
	l := &TLSListener{
		ctx:    ctx,
		cancel: cancel,
		tag:    tag,
		core:   core,
		logger: logger,
	}
	var err error
	l.listen, err = parseListen(options.Listen, 853)
	if err != nil {
		return nil, fmt.Errorf("create tls listener failed: %s", err)
	}
	if options.MaxConnection > 0 {
		l.maxConnection = options.MaxConnection
	} else {
		l.maxConnection = DefaultMaxConnection
	}
	if options.IdleTimeout > 0 {
		l.idleTimeout = time.Duration(options.IdleTimeout)
	} else {
		l.idleTimeout = DefaultIdleTimeout
	}
	if workflow == "" {
		return nil, fmt.Errorf("create tls listener failed: missing workflow")
	}
	l.workflowTag = workflow
	tlsConfig, err := newTLSConfig(options.TLSOptions)
	if err != nil {
		return nil, fmt.Errorf("create tls listener failed: %s", err)
	}
	l.tlsConfig = tlsConfig
	return l, nil
}

func (l *TLSListener) Tag() string {
	return l.tag
}

func (l *TLSListener) Type() string {
	return TLSListenerType
}

func (l *TLSListener) Start() error {
	w := l.core.GetWorkflow(l.workflowTag)
	if w == nil {
		return fmt.Errorf("create tls listener failed: workflow [%s] not found", l.workflowTag)
	}
	l.workflow = w
	l.limiter = utils.NewLimiter(l.maxConnection)
	var err error
	l.tlsListener, err = tls.Listen("tcp", l.listen, l.tlsConfig.Clone())
	if err != nil {
		return fmt.Errorf("listen tls failed: %s, error: %s", l.listen, err)
	}
	l.logger.Infof("tls listener: listen %s", l.listen)
	go l.loopHandle()
	return nil
}

func (l *TLSListener) Close() error {
	l.cancel()
	l.tlsListener.Close()
	return nil
}

func (l *TLSListener) loopHandle() {
	for {
		if !l.limiter.Get(l.ctx) {
			l.limiter.PutBack()
			return
		}
		conn, err := l.tlsListener.Accept()
		if err != nil {
			l.limiter.PutBack()
			return
		}
		go l.serve(conn)
	}
}

func (l *TLSListener) serve(conn net.Conn) {
	defer conn.Close()
	addr, err := netip.ParseAddrPort(conn.RemoteAddr().String())
	if err != nil {
		l.logger.Debugf("parse client address failed: %s", err)
		return
	}
	for {
		err = conn.SetReadDeadline(time.Now().Add(l.idleTimeout))
		if err != nil {
			if !connIsClosed(err) {
				l.logger.Errorf("set read deadline failed: %s", err)
			}
			return
		}
		var length uint16
		err = binary.Read(conn, binary.BigEndian, &length)
		if err != nil {
			if !connIsClosed(err) {
				l.logger.Errorf("read data failed: %s", err)
			}
			return
		}
		if length == 0 {
			if !connIsClosed(err) {
				l.logger.Error("invalid length")
			}
			return
		}
		data := make([]byte, length)
		_, err = conn.Read(data)
		if err != nil {
			if !connIsClosed(err) {
				l.logger.Errorf("read data failed: %s", err)
			}
			return
		}
		req := &dns.Msg{}
		err = req.Unpack(data)
		if err != nil {
			l.logger.Errorf("unpack dns message failed: %s", err)
			return
		}
		go func(req *dns.Msg) {
			resp := l.Handle(l.ctx, req, addr)
			if resp != nil {
				raw, err := resp.Pack()
				if err != nil {
					l.logger.Debugf("pack dns message failed: client address: %s, error: %s", addr.String(), err)
					return
				}
				buffer := make([]byte, 2+len(raw))
				binary.BigEndian.PutUint16(buffer, uint16(len(raw)))
				copy(buffer[2:], raw)
				err = conn.SetWriteDeadline(time.Now().Add(l.idleTimeout))
				if err != nil {
					l.logger.Errorf("set write deadline failed: %s", err)
					return
				}
				_, err = conn.Write(buffer)
				if err != nil {
					l.logger.Debugf("write dns message failed: client address: %s, error: %s", addr.String(), err)
				}
			}
		}(req)
	}
}

func (l *TLSListener) Handle(ctx context.Context, req *dns.Msg, clientAddr netip.AddrPort) *dns.Msg {
	return listenerHandle(ctx, l.tag, l.logger, l.workflow, req, clientAddr)
}
