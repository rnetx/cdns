package upstream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/miekg/dns"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"
)

const (
	DefaultConnectTimeout = 30 * time.Second
	DefaultIdleTimeout    = 60 * time.Second
	DefaultQueryTimeout   = 15 * time.Second
)

func reqMessageInfo(req *dns.Msg) string {
	questions := req.Question
	if len(questions) > 0 {
		return fmt.Sprintf("%s %s %s", dns.ClassToString[questions[0].Qclass], dns.TypeToString[questions[0].Qtype], questions[0].Name)
	}
	return "???"
}

func Exchange(ctx context.Context, req *dns.Msg, logger log.Logger, exchangeFunc func(ctx context.Context, req *dns.Msg) (*dns.Msg, error)) (resp *dns.Msg, err error) {
	messageInfo := reqMessageInfo(req)
	logger.InfoContext(ctx, "exchange: ", messageInfo)
	defer func() {
		if err != nil {
			logger.ErrorfContext(ctx, "exchange failed: %s, error: %s", messageInfo, err)
		} else {
			logger.InfoContext(ctx, "exchange success: ", messageInfo)
		}
	}()
	resp, err = exchangeFunc(ctx, req)
	return
}

type TLSOptions struct {
	Servername     string                 `yaml:"servername,omitempty"`
	Insecure       bool                   `yaml:"insecure,omitempty"`
	ServerCAFile   utils.Listable[string] `yaml:"server-ca-file,omitempty"`
	ClientCertFile string                 `yaml:"client-cert-file,omitempty"`
	ClientKeyFile  string                 `yaml:"client-key-file,omitempty"`
}

func newTLSConfig(options TLSOptions) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		ServerName:         options.Servername,
		InsecureSkipVerify: options.Insecure,
	}
	if len(options.ServerCAFile) > 0 {
		caPool := x509.NewCertPool()
		for _, caFile := range options.ServerCAFile {
			ca, err := os.ReadFile(caFile)
			if err != nil {
				return nil, fmt.Errorf("read server-ca-file failed: %s, error: %s", caFile, err)
			}
			if !caPool.AppendCertsFromPEM(ca) {
				return nil, fmt.Errorf("append server-ca-file failed: %s", caFile)
			}
		}
		tlsConfig.RootCAs = caPool
	}
	if (options.ClientCertFile == "" && options.ClientKeyFile != "") || (options.ClientCertFile != "" && options.ClientKeyFile == "") {
		return nil, fmt.Errorf("invalid client-cert-file or client-key-file")
	}
	if options.ClientCertFile != "" && options.ClientKeyFile != "" {
		certPair, err := tls.LoadX509KeyPair(options.ClientCertFile, options.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client-cert-file and client-key-file failed: %s", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certPair}
	}
	return tlsConfig, nil
}
