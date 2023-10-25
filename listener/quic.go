package listener

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

type QUICListenerOptions struct {
	Listen        string         `yaml:"listen"`
	IdleTimeout   utils.Duration `yaml:"idle-timeout,omitempty"`
	MaxConnection int            `yaml:"max-connection,omitempty"`
	Enable0RTT    bool           `yaml:"enable-0rtt,omitempty"`
	TLSOptions    TLSOptions     `yaml:",inline,omitempty"`
}

const QUICListenerType = "quic"

var (
	_ adapter.Listener = (*QUICListener)(nil)
	_ adapter.Starter  = (*QUICListener)(nil)
	_ adapter.Closer   = (*QUICListener)(nil)
)

type QUICListener struct {
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
	quicConfig    *quic.Config

	limiter           *utils.Limiter
	quicListener      *quic.Listener
	quicEarlyListener *quic.EarlyListener
}

func NewQUICListener(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options QUICListenerOptions, workflow string) (adapter.Listener, error) {
	ctx, cancel := context.WithCancel(ctx)
	l := &QUICListener{
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
	tlsConfig.NextProtos = []string{"doq"}
	l.tlsConfig = tlsConfig
	l.quicConfig = &quic.Config{
		Allow0RTT: options.Enable0RTT,
	}
	return l, nil
}

func (l *QUICListener) Tag() string {
	return l.tag
}

func (l *QUICListener) Type() string {
	return QUICListenerType
}

func (l *QUICListener) Start() error {
	w := l.core.GetWorkflow(l.workflowTag)
	if w == nil {
		return fmt.Errorf("create quic listener failed: workflow [%s] not found", l.workflowTag)
	}
	l.workflow = w
	l.limiter = utils.NewLimiter(l.maxConnection)
	var err error
	if l.quicConfig.Allow0RTT {
		l.quicEarlyListener, err = quic.ListenAddrEarly(l.listen, l.tlsConfig.Clone(), l.quicConfig.Clone())
	} else {
		l.quicListener, err = quic.ListenAddr(l.listen, l.tlsConfig.Clone(), l.quicConfig.Clone())
	}
	if err != nil {
		return fmt.Errorf("listen quic failed: %s, error: %s", l.listen, err)
	}
	l.logger.Infof("quic listener: listen %s", l.listen)
	go l.loopHandle()
	return nil
}

func (l *QUICListener) Close() error {
	l.cancel()
	if l.quicConfig.Allow0RTT {
		l.quicEarlyListener.Close()
	} else {
		l.quicListener.Close()
	}
	return nil
}

func (l *QUICListener) loopHandle() {
	var err error
	for {
		var quicConn quic.Connection
		if l.quicConfig.Allow0RTT {
			quicConn, err = l.quicEarlyListener.Accept(l.ctx)
		} else {
			quicConn, err = l.quicListener.Accept(l.ctx)
		}
		if err != nil {
			return
		}
		go l.serve(quicConn)
	}
}

func (l *QUICListener) serve(conn quic.Connection) {
	addr, err := netip.ParseAddrPort(conn.RemoteAddr().String())
	if err != nil {
		l.logger.Debugf("parse client address failed: %s", err)
		return
	}
	for {
		if !l.limiter.Get(l.ctx) {
			l.limiter.PutBack()
			conn.CloseWithError(0x4, "too many connections")
			return
		}
		ctx, cancel := context.WithTimeout(l.ctx, l.idleTimeout)
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			cancel()
			select {
			case <-conn.Context().Done():
				l.limiter.PutBack()
				conn.CloseWithError(0, "")
				return
			default:
			}
			if !errors.Is(err, os.ErrDeadlineExceeded) {
				l.logger.Errorf("accept stream failed: %s", err)
			}
			l.limiter.PutBack()
			conn.CloseWithError(0x5, "idle timeout")
			return
		}
		cancel()
		go l.serveStream(conn, stream, addr)
	}
}

func (l *QUICListener) serveStream(quicConn quic.Connection, stream quic.Stream, clientAddr netip.AddrPort) {
	defer l.limiter.PutBack()
	defer stream.Close()
	//
	err := stream.SetReadDeadline(time.Now().Add(l.idleTimeout))
	if err != nil {
		if !connIsClosed(err) {
			l.logger.Errorf("set read deadline failed: %s", err)
			quicConn.CloseWithError(0x5, "idle timeout")
		}
		return
	}
	var length uint16
	err = binary.Read(stream, binary.BigEndian, &length)
	if err != nil {
		if !connIsClosed(err) {
			l.logger.Errorf("read data failed: %s", err)
			quicConn.CloseWithError(0x5, "idle timeout")
		}
		return
	}
	if length == 0 {
		l.logger.Error("invalid length")
		quicConn.CloseWithError(0x2, "invalid length")
		return
	}
	data := make([]byte, length)
	var n int
	n, err = stream.Read(data)
	if err != nil && !errors.Is(err, io.EOF) {
		if !connIsClosed(err) {
			l.logger.Errorf("read data failed: %s", err)
			quicConn.CloseWithError(0x2, "invalid length")
		}
		return
	}
	req := &dns.Msg{}
	err = req.Unpack(data[:n])
	if err != nil {
		l.logger.Errorf("unpack dns message failed: client address: %s, error: %s", clientAddr.String(), err)
		return
	}
	id := dns.Id()
	req.Id = id // DOQ
	resp := l.Handle(l.ctx, req, clientAddr)
	if resp != nil {
		resp.Id = id // DOQ
		raw, err := resp.Pack()
		if err != nil {
			l.logger.Debugf("pack dns message failed: client address: %s, error: %s", clientAddr.String(), err)
			return
		}
		buffer := make([]byte, 2+len(raw))
		binary.BigEndian.PutUint16(buffer, uint16(len(raw)))
		copy(buffer[2:], raw)
		_, err = stream.Write(buffer)
		if err != nil {
			l.logger.Debugf("write dns message failed: client address: %s, error: %s", clientAddr.String(), err)
		}
	}
}

func (l *QUICListener) Handle(ctx context.Context, req *dns.Msg, clientAddr netip.AddrPort) *dns.Msg {
	return listenerHandle(ctx, l.listen, l.logger, l.workflow, req, clientAddr)
}
