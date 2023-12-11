package upstream

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
)

func init() {
	os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true") // QUIC GSO Setting
}

type Options struct {
	Tag          string
	Type         string
	QueryTimeout time.Duration

	UDPOptions   *UDPUpstreamOptions
	TCPOptions   *TCPUpstreamOptions
	TLSOptions   *TLSUpstreamOptions
	HTTPSOptions *HTTPSUpstreamOptions
	QUICOptions  *QUICUpstreamOptions

	HostsOptions *HostsUpstreamOptions
	DHCPOptions  *DHCPUpstreamOptions

	RandomOptions    *RandomUpstreamOptions
	ParallelOptions  *ParallelUpstreamOptions
	QueryTestOptions *QueryTestUpstreamOptions
	FallbackOptions  *FallbackUpstreamOptions
}

type _Options struct {
	Tag          string         `yaml:"tag"`
	Type         string         `yaml:"type"`
	QueryTimeout utils.Duration `yaml:"query-timeout"`
}

func (o *Options) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var _o _Options
	err := unmarshal(&_o)
	if err != nil {
		return err
	}
	var data any
	switch _o.Type {
	case UDPUpstreamType:
		o.UDPOptions = &UDPUpstreamOptions{}
		data = o.UDPOptions
	case TCPUpstreamType:
		o.TCPOptions = &TCPUpstreamOptions{}
		data = o.TCPOptions
	case TLSUpstreamType:
		o.TLSOptions = &TLSUpstreamOptions{}
		data = o.TLSOptions
	case HTTPSUpstreamType:
		o.HTTPSOptions = &HTTPSUpstreamOptions{}
		data = o.HTTPSOptions
	case QUICUpstreamType:
		o.QUICOptions = &QUICUpstreamOptions{}
		data = o.QUICOptions
	case HostsUpstreamType:
		o.HostsOptions = &HostsUpstreamOptions{}
		data = o.HostsOptions
	case DHCPUpstreamType:
		o.DHCPOptions = &DHCPUpstreamOptions{}
		data = o.DHCPOptions
	case RandomUpstreamType:
		o.RandomOptions = &RandomUpstreamOptions{}
		data = o.RandomOptions
	case ParallelUpstreamType:
		o.ParallelOptions = &ParallelUpstreamOptions{}
		data = o.ParallelOptions
	case QueryTestUpstreamType:
		o.QueryTestOptions = &QueryTestUpstreamOptions{}
		data = o.QueryTestOptions
	case FallbackUpstreamType:
		o.FallbackOptions = &FallbackUpstreamOptions{}
		data = o.FallbackOptions
	default:
		return fmt.Errorf("unknown upstream type: %s", _o.Type)
	}
	err = unmarshal(data)
	if err != nil {
		return err
	}
	o.Type = _o.Type
	o.Tag = _o.Tag
	o.QueryTimeout = time.Duration(_o.QueryTimeout)
	return nil
}

const DefaultRetry = 3

type GenericUpstream struct {
	adapter.Upstream
	queryTimeout time.Duration
	retry        int
}

func (g *GenericUpstream) Start() error {
	starter, isStarter := g.Upstream.(adapter.Starter)
	if isStarter {
		return starter.Start()
	}
	return nil
}

func (g *GenericUpstream) Close() error {
	closer, isCloser := g.Upstream.(adapter.Closer)
	if isCloser {
		return closer.Close()
	}
	return nil
}

func (g *GenericUpstream) Exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(g.queryTimeout))
	defer cancel()
	var lastErr error
	for i := 0; i < g.retry; i++ {
		resp, err := g.Upstream.Exchange(ctx, req)
		if err == nil {
			return resp, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			lastErr = err
		}
	}
	return nil, lastErr
}

func NewUpstream(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options Options) (adapter.Upstream, error) {
	var (
		u         adapter.Upstream
		noGeneric bool
		err       error
	)
	switch options.Type {
	case UDPUpstreamType:
		u, err = NewUDPUpstream(ctx, core, logger, tag, *options.UDPOptions)
	case TCPUpstreamType:
		u, err = NewTCPUpstream(ctx, core, logger, tag, *options.TCPOptions)
	case TLSUpstreamType:
		u, err = NewTLSUpstream(ctx, core, logger, tag, *options.TLSOptions)
	case HTTPSUpstreamType:
		u, err = NewHTTPSUpstream(ctx, core, logger, tag, *options.HTTPSOptions)
	case QUICUpstreamType:
		u, err = NewQUICUpstream(ctx, core, logger, tag, *options.QUICOptions)
	case HostsUpstreamType:
		noGeneric = true
		u, err = NewHostsUpstream(ctx, core, logger, tag, *options.HostsOptions)
	case DHCPUpstreamType:
		u, err = NewDHCPUpstream(ctx, core, logger, tag, *options.DHCPOptions)
	case RandomUpstreamType:
		noGeneric = true
		u, err = NewRandomUpstream(ctx, core, logger, tag, *options.RandomOptions)
	case ParallelUpstreamType:
		noGeneric = true
		u, err = NewParallelUpstream(ctx, core, logger, tag, *options.ParallelOptions)
	case QueryTestUpstreamType:
		noGeneric = true
		u, err = NewQueryTestUpstream(ctx, core, logger, tag, *options.QueryTestOptions)
	case FallbackUpstreamType:
		noGeneric = true
		u, err = NewFallbackUpstream(ctx, core, logger, tag, *options.FallbackOptions)
	default:
		return nil, fmt.Errorf("unknown upstream type: %s", options.Type)
	}
	if err != nil {
		return nil, err
	}
	if !noGeneric {
		queryTimeout := options.QueryTimeout
		if queryTimeout <= 0 {
			queryTimeout = DefaultQueryTimeout
		}
		u = &GenericUpstream{
			queryTimeout: queryTimeout,
			retry:        DefaultRetry,
			Upstream:     u,
		}
	}
	return u, nil
}
