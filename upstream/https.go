package upstream

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/upstream/bootstrap"
	"github.com/rnetx/cdns/upstream/network"
	"github.com/rnetx/cdns/upstream/network/common"
	"github.com/rnetx/cdns/utils"
)

type HTTPSUpstreamOptions struct {
	Address          string             `yaml:"address"`
	ConnectTimeout   utils.Duration     `yaml:"connect-timeout,omitempty"`
	IdleTimeout      utils.Duration     `yaml:"idle-timeout,omitempty"`
	TLSOptions       TLSOptions         `yaml:",inline,omitempty"`
	UseHTTP3         bool               `yaml:"use-http3,omitempty"`
	UsePost          bool               `yaml:"use-post,omitempty"`
	Path             string             `yaml:"path,omitempty"`
	Headers          map[string]string  `yaml:"headers,omitempty"`
	BootstrapOptions *bootstrap.Options `yaml:"bootstrap,omitempty"`
	DialerOptions    network.Options    `yaml:",inline,omitempty"`
}

const (
	DefaultHTTPSPath      = "/dns-query"
	DefaultHTTPSUserAgent = "cdns"
)

const HTTPSUpstreamType = "https"

var (
	_ adapter.Upstream = (*HTTPSUpstream)(nil)
	_ adapter.Starter  = (*HTTPSUpstream)(nil)
	_ adapter.Closer   = (*HTTPSUpstream)(nil)
)

type HTTPSUpstream struct {
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
	useHTTP3  bool
	usePost   bool
	url       url.URL
	headers   http.Header

	httpClient     *http.Client
	httpTransport  *http.Transport
	http3Transport *http3.RoundTripper
}

func NewHTTPSUpstream(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options HTTPSUpstreamOptions) (adapter.Upstream, error) {
	u := &HTTPSUpstream{
		ctx:    ctx,
		tag:    tag,
		core:   core,
		logger: logger,
	}
	socksAddr, err := common.NewSocksAddrFromStringWithDefaultPort(options.Address, 443)
	if err != nil {
		return nil, fmt.Errorf("create https upstream failed: invalid address: %s, error: %s", options.Address, err)
	}
	u.address = *socksAddr
	dialer, err := network.NewDialer(options.DialerOptions)
	if err != nil {
		return nil, fmt.Errorf("create https upstream failed: create dialer: %s", err)
	}
	u.dialer = dialer
	if options.BootstrapOptions != nil {
		b, err := bootstrap.NewBootstrap(ctx, core, *options.BootstrapOptions)
		if err != nil {
			return nil, fmt.Errorf("create https upstream failed: create bootstrap: %s", err)
		}
		u.bootstrap = b
	}
	if u.address.IsDomain() && !network.IsSocks5Dialer(u.dialer) && u.bootstrap == nil {
		return nil, fmt.Errorf("create https upstream failed: domain address requires socks5 dialer or bootstrap")
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
	u.useHTTP3 = options.UseHTTP3
	u.usePost = options.UsePost
	var host string
	if options.Headers != nil && len(options.Headers) > 0 {
		headers := make(http.Header)
		for k, v := range options.Headers {
			headers.Set(k, v)
		}
		u.headers = headers
		host = headers.Get("Host")
	}
	var urlHost string
	if host != "" {
		port := u.address.Port()
		if port != 443 {
			urlHost = net.JoinHostPort(host, strconv.Itoa(int(u.address.Port())))
		} else {
			urlHost = host
		}
	} else {
		port := u.address.Port()
		if port != 443 {
			urlHost = u.address.String()
		} else {
			if u.address.IsDomain() {
				urlHost = u.address.Domain()
			} else if u.address.IsIPv4() {
				urlHost = u.address.IP().String()
			} else {
				urlHost = fmt.Sprintf("[%s]", u.address.IP().String())
			}
		}
	}
	path := options.Path
	if path == "" {
		path = DefaultHTTPSPath
	}
	var query string
	if strings.Contains(path, "?") {
		s := strings.SplitN(path, "?", 2)
		path = s[0]
		query = s[1]
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimSuffix(path, "/")
	uri := &url.URL{
		Scheme: "https",
		Host:   urlHost,
	}
	uri.Path = path
	uri.RawQuery = query
	if uri.RawQuery != "" {
		q := uri.Query()
		q.Del("dns")
		uri.RawQuery = q.Encode()
	}
	u.url = *uri
	tlsConfig, err := newTLSConfig(options.TLSOptions)
	if err != nil {
		return nil, fmt.Errorf("create https upstream failed: create tls config: %s", err)
	}
	if tlsConfig.ServerName == "" {
		if host != "" {
			tlsConfig.ServerName = host
		} else {
			if u.address.IsDomain() {
				tlsConfig.ServerName = u.address.Domain()
			} else {
				tlsConfig.ServerName = u.address.IP().String()
			}
		}
	}
	if u.useHTTP3 {
		tlsConfig.NextProtos = []string{"h3", "dns"}
	} else {
		tlsConfig.NextProtos = []string{"h2", "http/1.1", "dns"}
	}
	u.tlsConfig = tlsConfig
	return u, nil
}

func (u *HTTPSUpstream) Tag() string {
	return u.tag
}

func (u *HTTPSUpstream) Type() string {
	return HTTPSUpstreamType
}

func (u *HTTPSUpstream) Dependencies() []string {
	if u.bootstrap != nil {
		return []string{u.bootstrap.UpstreamTag()}
	}
	return nil
}

func (u *HTTPSUpstream) Start() error {
	if u.bootstrap != nil {
		err := u.bootstrap.Start()
		if err != nil {
			return fmt.Errorf("start bootstrap failed: %s", err)
		}
	}
	var httpTransport http.RoundTripper
	if !u.useHTTP3 {
		u.httpTransport = &http.Transport{
			ForceAttemptHTTP2:   true,
			IdleConnTimeout:     u.idleTimeout,
			MaxIdleConnsPerHost: 16,
			TLSClientConfig:     u.tlsConfig.Clone(),
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				conn, err := u.newTCPConn(ctx)
				if err != nil {
					return nil, err
				}
				u.logger.Debug("new tcp connection")
				return newHTTPTCPConn(conn, u.logger), nil
			},
		}
		httpTransport = u.httpTransport
	} else {
		u.http3Transport = &http3.RoundTripper{
			TLSClientConfig: u.tlsConfig.Clone(),
			QuicConfig: &quic.Config{
				MaxIdleTimeout: u.idleTimeout,
			},
			Dial: func(ctx context.Context, _ string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error) {
				udpConn, remoteAddr, err := u.newUDPPacketConn(ctx)
				if err != nil {
					return nil, err
				}
				quicConnection, err := quic.DialEarly(ctx, udpConn, remoteAddr, tlsCfg, cfg)
				if err != nil {
					return nil, err
				}
				u.logger.Debug("new quic connection")
				return newHTTPQUICEarlyConn(quicConnection, u.logger), nil
			},
		}
		httpTransport = u.http3Transport
	}
	u.httpClient = &http.Client{
		Transport: httpTransport,
	}
	return nil
}

func (u *HTTPSUpstream) Close() error {
	if u.bootstrap != nil {
		u.bootstrap.Close()
	}
	if !u.useHTTP3 {
		u.httpTransport.CloseIdleConnections()
	} else {
		u.http3Transport.CloseIdleConnections()
		u.http3Transport.Close()
	}
	return nil
}

func (u *HTTPSUpstream) newTCPConn(ctx context.Context) (net.Conn, error) {
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

func (u *HTTPSUpstream) newUDPPacketConn(ctx context.Context) (net.PacketConn, net.Addr, error) {
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

func (u *HTTPSUpstream) newGETRequest(req *dns.Msg) (*http.Request, error) {
	raw, err := req.Pack()
	if err != nil {
		return nil, fmt.Errorf("pack dns message failed: %s", err)
	}
	uri := u.url
	q := uri.Query()
	q.Add("dns", base64.RawURLEncoding.EncodeToString(raw))
	uri.RawQuery = q.Encode()
	httpReq, err := http.NewRequest(http.MethodGet, uri.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create http request failed: %s", err)
	}
	if u.headers != nil {
		httpReq.Header = u.headers.Clone()
	}
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", DefaultHTTPSUserAgent)
	}
	httpReq.Header.Set("Content-Type", "application/dns-message")
	httpReq.Header.Set("Accept", "application/dns-message")
	return httpReq, nil
}

func (u *HTTPSUpstream) newPOSTRequest(req *dns.Msg) (*http.Request, error) {
	raw, err := req.Pack()
	if err != nil {
		return nil, fmt.Errorf("pack dns message failed: %s", err)
	}
	uri := u.url
	httpReq, err := http.NewRequest(http.MethodPost, uri.String(), bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("create http request failed: %s", err)
	}
	if u.headers != nil {
		httpReq.Header = u.headers.Clone()
	}
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", DefaultHTTPSUserAgent)
	}
	httpReq.Header.Set("Content-Type", "application/dns-message")
	httpReq.Header.Set("Accept", "application/dns-message")
	return httpReq, nil
}

func (u *HTTPSUpstream) newHTTPRequest(req *dns.Msg) (*http.Request, error) {
	if !u.usePost {
		return u.newGETRequest(req)
	} else {
		return u.newPOSTRequest(req)
	}
}

func (u *HTTPSUpstream) exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	httpReq, err := u.newHTTPRequest(req)
	if err != nil {
		return nil, err
	}

	httpResp, err := u.httpClient.Do(httpReq.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("send http request failed: %s", err)
	}
	buffer := bytes.NewBuffer(nil)
	_, err = io.Copy(buffer, httpResp.Body)
	httpResp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read http response failed: %s", err)
	}

	resp := &dns.Msg{}
	err = resp.Unpack(buffer.Bytes())
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (u *HTTPSUpstream) Exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	newReq := new(dns.Msg)
	*newReq = *req
	newReq.Id = 0
	resp, err := Exchange(ctx, newReq, u.logger, u.exchange)
	if err == nil {
		resp.Id = req.Id
	}
	return resp, err
}

type HTTPTCPConn struct {
	net.Conn
	logger   log.Logger
	isClosed bool
}

func newHTTPTCPConn(conn net.Conn, logger log.Logger) *HTTPTCPConn {
	return &HTTPTCPConn{
		Conn:   conn,
		logger: logger,
	}
}

func (c *HTTPTCPConn) Close() error {
	if !c.isClosed {
		c.logger.Debug("close tcp connection")
		c.isClosed = true
	}
	return c.Conn.Close()
}

type HTTPQUICEarlyConn struct {
	quic.EarlyConnection
	logger   log.Logger
	isClosed bool
}

func newHTTPQUICEarlyConn(conn quic.EarlyConnection, logger log.Logger) *HTTPQUICEarlyConn {
	return &HTTPQUICEarlyConn{
		EarlyConnection: conn,
		logger:          logger,
	}
}

func (c *HTTPQUICEarlyConn) CloseWithError(code quic.ApplicationErrorCode, reason string) error {
	if !c.isClosed {
		c.logger.Debug("close quic early connection")
		c.isClosed = true
	}
	return c.EarlyConnection.CloseWithError(code, reason)
}

func (c *HTTPQUICEarlyConn) NextConnection() quic.Connection {
	return newHTTPQUICConn(c.EarlyConnection.NextConnection(), c.logger)
}

type HTTPQUICConn struct {
	quic.Connection
	logger   log.Logger
	isClosed bool
}

func newHTTPQUICConn(conn quic.Connection, logger log.Logger) *HTTPQUICConn {
	return &HTTPQUICConn{
		Connection: conn,
		logger:     logger,
	}
}

func (c *HTTPQUICConn) CloseWithError(code quic.ApplicationErrorCode, reason string) error {
	if !c.isClosed {
		c.logger.Debug("close quic connection")
		c.isClosed = true
	}
	return c.Connection.CloseWithError(code, reason)
}
