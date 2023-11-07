package listener

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

type HTTPListenerOptions struct {
	Listen       string                 `yaml:"listen"`
	Path         string                 `yaml:"path"`
	ReadIPHeader string                 `yaml:"read-ip-header,omitempty"`
	TrustIP      utils.Listable[string] `yaml:"trust-ip,omitempty"`
	UseHTTP3     bool                   `yaml:"use-http3,omitempty"`
	Enable0RTT   bool                   `yaml:"enable-0rtt,omitempty"`
	TLSOptions   *TLSOptions            `yaml:",inline,omitempty"`
}

const (
	HTTPListenerType        = "http"
	HTTPListenerDefaultPath = "/dns-query"
)

var (
	_ adapter.Listener = (*HTTPListener)(nil)
	_ adapter.Starter  = (*HTTPListener)(nil)
	_ adapter.Closer   = (*HTTPListener)(nil)
)

type HTTPListener struct {
	ctx    context.Context
	cancel context.CancelFunc
	tag    string
	core   adapter.Core
	logger log.Logger

	listen       string
	workflowTag  string
	workflow     adapter.Workflow
	useHTTP3     bool
	path         string
	realIPHeader string
	trustIP      []netip.Prefix

	tlsConfig  *tls.Config
	enable0RTT bool

	listener     net.Listener
	quicListener *quic.EarlyListener
}

func NewHTTPListener(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options HTTPListenerOptions, workflow string) (adapter.Listener, error) {
	ctx, cancel := context.WithCancel(ctx)
	l := &HTTPListener{
		ctx:    ctx,
		cancel: cancel,
		tag:    tag,
		core:   core,
		logger: logger,
	}
	var err error
	if options.TLSOptions != nil {
		l.listen, err = parseListen(options.Listen, 443)
	} else {
		l.listen, err = parseListen(options.Listen, 80)
	}
	if err != nil {
		return nil, fmt.Errorf("create http listener failed: %s", err)
	}
	if options.TLSOptions != nil {
		tlsConfig, err := newTLSConfig(*options.TLSOptions)
		if err != nil {
			return nil, fmt.Errorf("create http listener failed: %s", err)
		}
		if !options.UseHTTP3 {
			tlsConfig.NextProtos = []string{"h2", "http/1.1", "dns"}
		} else {
			tlsConfig.NextProtos = []string{"h3", "dns"}
		}
		l.tlsConfig = tlsConfig
	}
	l.useHTTP3 = options.UseHTTP3
	if l.useHTTP3 && l.tlsConfig == nil {
		return nil, fmt.Errorf("create http listener failed: missing tls config, use-http3 must be used with tls")
	}
	l.realIPHeader = options.ReadIPHeader
	if options.Path != "" {
		l.path = options.Path
	} else {
		l.path = HTTPListenerDefaultPath
	}
	if !strings.HasPrefix(l.path, "/") {
		l.path = "/" + l.path
	}
	l.path = strings.TrimSuffix(l.path, "/")
	if len(options.TrustIP) > 0 {
		trustIP := make([]netip.Prefix, 0, len(options.TrustIP))
		for _, s := range options.TrustIP {
			prefix, err := netip.ParsePrefix(s)
			if err == nil {
				trustIP = append(trustIP, prefix)
				continue
			}
			ip, err := netip.ParseAddr(s)
			if err == nil {
				bits := 0
				if ip.Is4() {
					bits = 32
				} else {
					bits = 128
				}
				trustIP = append(trustIP, netip.PrefixFrom(ip, bits))
				continue
			}
			return nil, fmt.Errorf("create http listener failed: invalid trust-ip: %s", s)
		}
		l.trustIP = trustIP
	}
	l.enable0RTT = options.Enable0RTT
	if workflow == "" {
		return nil, fmt.Errorf("create http listener failed: missing workflow")
	}
	l.workflowTag = workflow
	return l, nil
}

func (l *HTTPListener) Tag() string {
	return l.tag
}

func (l *HTTPListener) Type() string {
	return HTTPListenerType
}

func (l *HTTPListener) Start() error {
	w := l.core.GetWorkflow(l.workflowTag)
	if w == nil {
		return fmt.Errorf("create http listener failed: workflow [%s] not found", l.workflowTag)
	}
	l.workflow = w
	var err error
	if !l.useHTTP3 {
		httpServer := &http.Server{
			Handler: l.newHTTPHandle(),
		}
		if l.tlsConfig == nil {
			l.listener, err = net.Listen("tcp", l.listen)
		} else {
			l.listener, err = tls.Listen("tcp", l.listen, l.tlsConfig.Clone())
		}
		if err != nil {
			if l.tlsConfig == nil {
				return fmt.Errorf("listen http failed: %s, error: %s", l.listen, err)
			} else {
				return fmt.Errorf("listen https failed: %s, error: %s", l.listen, err)
			}
		}
		go httpServer.Serve(l.listener)
	} else {
		http3Server := &http3.Server{
			Handler: l.newHTTPHandle(),
		}
		l.quicListener, err = quic.ListenAddrEarly(l.listen, l.tlsConfig, &quic.Config{
			Allow0RTT: l.enable0RTT,
		})
		if err != nil {
			return fmt.Errorf("listen http3 failed: %s, error: %s", l.listen, err)
		}
		go http3Server.ServeListener(l.quicListener)
	}
	if !l.useHTTP3 {
		if l.tlsConfig == nil {
			l.logger.Infof("http listener: listen %s", l.listen)
		} else {
			l.logger.Infof("https listener: listen %s", l.listen)
		}
	} else {
		l.logger.Infof("http3 listener: listen %s", l.listen)
	}
	return nil
}

func (l *HTTPListener) Close() error {
	l.cancel()
	if l.quicListener != nil {
		l.quicListener.Close()
	} else {
		l.listener.Close()
	}
	return nil
}

func (l *HTTPListener) newHTTPHandle() http.Handler {
	if l.path == "/" {
		return http.Handler(l)
	}
	mux := http.NewServeMux()
	mux.Handle(l.path, http.Handler(l))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	return mux
}

func (l *HTTPListener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	addr, ok := l.realIP(w, r)
	if !ok {
		return
	}
	clientAddr := netip.AddrPortFrom(addr, 0)
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req *dns.Msg
	switch r.Method {
	case http.MethodPost:
		if r.Header.Get("Content-Type") != "application/dns-message" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		buffer := bytes.NewBuffer(nil)
		_, err := buffer.ReadFrom(r.Body)
		r.Body.Close()
		if err != nil {
			l.logger.Debugf("read http body failed: client address: %s, error: %s", clientAddr.String(), err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		req = &dns.Msg{}
		err = req.Unpack(buffer.Bytes())
		if err != nil {
			l.logger.Debugf("unpack dns message failed: client address: %s, error: %s", clientAddr.String(), err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	case http.MethodGet:
		q := r.URL.Query().Get("dns")
		raw, err := base64.RawURLEncoding.DecodeString(q)
		if err != nil {
			l.logger.Debugf("decode dns message failed: client address: %s, error: %s", clientAddr.String(), err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		req = &dns.Msg{}
		err = req.Unpack(raw)
		if err != nil {
			l.logger.Debugf("unpack dns message failed: client address: %s, error: %s", clientAddr.String(), err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	oldID := req.Id
	req.Id = dns.Id() // DOH
	resp := l.Handle(l.ctx, req, clientAddr)
	if resp != nil {
		resp.Id = oldID // DOH
		raw, err := resp.Pack()
		if err != nil {
			l.logger.Debugf("pack dns message failed: client address: %s, error: %s", clientAddr.String(), err)
			return
		}
		w.Header().Set("Content-Type", "application/dns-message")
		w.WriteHeader(http.StatusOK)
		w.Write(raw)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (l *HTTPListener) realIP(w http.ResponseWriter, r *http.Request) (netip.Addr, bool) {
	addr, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		l.logger.Debugf("parse client address failed: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return netip.Addr{}, false
	}
	ip := addr.Addr()
	var realIP netip.Addr
	var realIPStr string
	if l.realIPHeader != "" {
		match := false
		if len(l.trustIP) > 0 {
			for _, p := range l.trustIP {
				if p.Contains(ip) {
					match = true
					break
				}
			}
		} else {
			match = true
		}
		if match {
			realIPStr = r.Header.Get(l.realIPHeader)
			if realIPStr != "" {
				realIP, err = netip.ParseAddr(realIPStr)
				if err != nil {
					l.logger.Debugf("parse real ip from header failed: %s", err)
					w.WriteHeader(http.StatusBadRequest)
					return netip.Addr{}, false
				}
			}
		}
	}
	if !realIP.IsValid() && l.path != "X-Real-IP" {
		realIPStr = r.Header.Get("X-Real-IP")
		if realIPStr != "" {
			realIP, err = netip.ParseAddr(realIPStr)
			if err != nil {
				l.logger.Debugf("parse real ip from header failed: %s", err)
				w.WriteHeader(http.StatusBadRequest)
				return netip.Addr{}, false
			}
		}
	}
	if !realIP.IsValid() {
		realIPStr = r.Header.Get("X-Forwarded-For")
		if realIPStr != "" {
			ss := strings.Split(realIPStr, ",")
			if len(ss) > 0 {
				s := ss[0]
				s = strings.TrimSpace(s)
				realIP, err = netip.ParseAddr(s)
				if err != nil {
					l.logger.Debugf("parse real ip from header failed: %s", err)
					w.WriteHeader(http.StatusBadRequest)
					return netip.Addr{}, false
				}
			}
		}
	}
	if !realIP.IsValid() {
		realIP = ip
	}
	return realIP, true
}

func (l *HTTPListener) Handle(ctx context.Context, req *dns.Msg, clientAddr netip.AddrPort) *dns.Msg {
	return listenerHandle(ctx, l.tag, l.logger, l.workflow, req, clientAddr)
}
