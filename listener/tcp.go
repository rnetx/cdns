package listener

import (
	"context"
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

type TCPListenerOptions struct {
	Listen        string         `yaml:"listen"`
	IdleTimeout   utils.Duration `yaml:"idle-timeout,omitempty"`
	MaxConnection int            `yaml:"max-connection,omitempty"`
}

const TCPListenerType = "tcp"

var (
	_ adapter.Listener = (*TCPListener)(nil)
	_ adapter.Starter  = (*TCPListener)(nil)
	_ adapter.Closer   = (*TCPListener)(nil)
)

type TCPListener struct {
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

	limiter     *utils.Limiter
	tcpListener net.Listener
}

func NewTCPListener(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options TCPListenerOptions, workflow string) (adapter.Listener, error) {
	ctx, cancel := context.WithCancel(ctx)
	l := &TCPListener{
		ctx:    ctx,
		cancel: cancel,
		tag:    tag,
		core:   core,
		logger: logger,
	}
	var err error
	l.listen, err = parseListen(options.Listen, 53)
	if err != nil {
		return nil, fmt.Errorf("create tcp listener failed: %s", err)
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
		return nil, fmt.Errorf("create tcp listener failed: missing workflow")
	}
	l.workflowTag = workflow
	return l, nil
}

func (l *TCPListener) Tag() string {
	return l.tag
}

func (l *TCPListener) Type() string {
	return TCPListenerType
}

func (l *TCPListener) Start() error {
	w := l.core.GetWorkflow(l.workflowTag)
	if w == nil {
		return fmt.Errorf("create tcp listener failed: workflow [%s] not found", l.workflowTag)
	}
	l.workflow = w
	l.limiter = utils.NewLimiter(l.maxConnection)
	var err error
	l.tcpListener, err = net.Listen("tcp", l.listen)
	if err != nil {
		return fmt.Errorf("listen tcp failed: %s, error: %s", l.listen, err)
	}
	l.logger.Infof("tcp listener: listen %s", l.listen)
	go l.loopHandle()
	return nil
}

func (l *TCPListener) Close() error {
	l.cancel()
	l.tcpListener.Close()
	return nil
}

func (l *TCPListener) loopHandle() {
	for {
		if !l.limiter.Get(l.ctx) {
			l.limiter.PutBack()
			return
		}
		conn, err := l.tcpListener.Accept()
		if err != nil {
			l.limiter.PutBack()
			return
		}
		go l.serve(conn)
	}
}

func (l *TCPListener) serve(conn net.Conn) {
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

func (l *TCPListener) Handle(ctx context.Context, req *dns.Msg, clientAddr netip.AddrPort) *dns.Msg {
	return listenerHandle(ctx, l.listen, l.logger, l.workflow, req, clientAddr)
}
