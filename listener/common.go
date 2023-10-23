package listener

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"
)

const (
	DefaultIdleTimeout   = 60 * time.Second
	DefaultDealTimeout   = 20 * time.Second
	DefaultMaxConnection = 256
)

func parseListen(listen string, defaultPort uint16) (string, error) {
	addr, err := netip.ParseAddrPort(listen)
	if err == nil {
		return addr.String(), nil
	}
	_listen := listen
	_listen = strings.Trim(_listen, "[]")
	ip, err := netip.ParseAddr(_listen)
	if err == nil {
		return netip.AddrPortFrom(ip, defaultPort).String(), nil
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("invalid listen: %s, error: %s", listen, err)
	}
	if host == "" {
		host = "::"
	}
	ip, err = netip.ParseAddr(host)
	if err != nil {
		return "", fmt.Errorf("invalid listen: %s, error: %s", listen, err)
	}
	portUint16, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return "", fmt.Errorf("invalid listen: %s, error: %s", listen, err)
	}
	if portUint16 == 0 {
		return "", fmt.Errorf("invalid listen: %s, error: invalid port", listen)
	}
	return net.JoinHostPort(ip.String(), strconv.FormatUint(portUint16, 10)), nil
}

type TLSOptions struct {
	ClientCAFile   utils.Listable[string] `yaml:"client-ca-file,omitempty"`
	ServerCertFile string                 `yaml:"server-cert-file,omitempty"`
	ServerKeyFile  string                 `yaml:"server-key-file,omitempty"`
}

func newTLSConfig(options TLSOptions) (*tls.Config, error) {
	tlsConfig := &tls.Config{}
	if options.ServerCertFile == "" && options.ServerKeyFile == "" {
		return nil, fmt.Errorf("server-cert-file and server-key-file must be set")
	} else if options.ServerCertFile != "" && options.ServerKeyFile == "" {
		return nil, fmt.Errorf("server-key-file must be set")
	} else if options.ServerCertFile == "" && options.ServerKeyFile != "" {
		return nil, fmt.Errorf("server-cert-file must be set")
	} else {
		cert, err := tls.LoadX509KeyPair(options.ServerCertFile, options.ServerKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load server-cert-file and server-key-file failed: %s", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	if options.ClientCAFile != nil && len(options.ClientCAFile) > 0 {
		caPool := x509.NewCertPool()
		for _, caFile := range options.ClientCAFile {
			ca, err := os.ReadFile(caFile)
			if err != nil {
				return nil, fmt.Errorf("read client-ca-file failed: %s, error: %s", caFile, err)
			}
			if !caPool.AppendCertsFromPEM(ca) {
				return nil, fmt.Errorf("append client-ca-file failed: %s", caFile)
			}
		}
		tlsConfig.ClientCAs = caPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return tlsConfig, nil
}

type GenericListener struct {
	adapter.Listener
	dealTimeout time.Duration
}

func (l *GenericListener) Start() error {
	starter, isStarter := l.Listener.(adapter.Starter)
	if isStarter {
		return starter.Start()
	}
	return nil
}

func (l *GenericListener) Close() error {
	closer, isCloser := l.Listener.(adapter.Closer)
	if isCloser {
		return closer.Close()
	}
	return nil
}

func (l *GenericListener) Handle(ctx context.Context, req *dns.Msg, clientAddr netip.AddrPort) *dns.Msg {
	ctx, cancel := context.WithTimeout(ctx, l.dealTimeout)
	defer cancel()
	return l.Listener.Handle(ctx, req, clientAddr)
}

func reqMessageInfo(req *dns.Msg) string {
	questions := req.Question
	if len(questions) > 0 {
		return fmt.Sprintf("%s %s %s", dns.ClassToString[questions[0].Qclass], dns.TypeToString[questions[0].Qtype], questions[0].Name)
	}
	return "???"
}

func listenerHandle(ctx context.Context, listener string, logger log.Logger, workflow adapter.Workflow, req *dns.Msg, clientAddr netip.AddrPort) *dns.Msg {
	dnsCtx := adapter.NewDNSContext(ctx, listener, clientAddr.Addr(), req)
	ctx = dnsCtx.Context()
	logger.DebugfContext(ctx, "new request: %s", reqMessageInfo(req))
	defer func() {
		err := recover()
		if err != nil {
			logger.FatalfContext(ctx, "handle request failed: %s, error(painc): %s", reqMessageInfo(req), err)
		}
	}()
	_, err := workflow.Exec(ctx, dnsCtx)
	if err != nil {
		return nil
	}
	resp := dnsCtx.RespMsg()
	if resp == nil {
		// Empty Response
		resp = &dns.Msg{}
		resp.SetRcode(req, dns.RcodeServerFailure)
	}
	return resp
}
