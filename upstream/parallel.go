package upstream

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
)

type ParallelUpstreamOptions struct {
	Upstreams []string `yaml:"upstreams"`
}

const ParallelUpstreamType = "parallel"

var (
	_ adapter.Upstream = (*ParallelUpstream)(nil)
	_ adapter.Starter  = (*ParallelUpstream)(nil)
)

type ParallelUpstream struct {
	tag    string
	core   adapter.Core
	logger log.Logger

	upstreamTags []string
	upstreams    []adapter.Upstream

	reqTotal   atomic.Uint64
	reqSuccess atomic.Uint64
}

func NewParallelUpstream(_ context.Context, core adapter.Core, logger log.Logger, tag string, options ParallelUpstreamOptions) (adapter.Upstream, error) {
	u := &ParallelUpstream{
		tag:          tag,
		core:         core,
		logger:       logger,
		upstreamTags: options.Upstreams,
	}
	if len(u.upstreamTags) == 0 {
		return nil, fmt.Errorf("create parallel upstream failed: missing upstreams")
	}
	return u, nil
}

func (u *ParallelUpstream) Tag() string {
	return u.tag
}

func (u *ParallelUpstream) Type() string {
	return ParallelUpstreamType
}

func (u *ParallelUpstream) Dependencies() []string {
	return u.upstreamTags
}

func (u *ParallelUpstream) Start() error {
	u.upstreams = make([]adapter.Upstream, 0, len(u.upstreamTags))
	for _, tag := range u.upstreamTags {
		uu := u.core.GetUpstream(tag)
		if uu == nil {
			return fmt.Errorf("upstream [%s] not found", tag)
		}
		u.upstreams = append(u.upstreams, uu)
	}
	return nil
}

func (u *ParallelUpstream) exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := utils.NewSafeChan[utils.Result[*dns.Msg]](len(u.upstreams))
	defer ch.Close()
	for _, uu := range u.upstreams {
		go func(uu adapter.Upstream, ch *utils.SafeChan[utils.Result[*dns.Msg]]) {
			defer ch.Close()
			resp, err := uu.Exchange(ctx, req)
			if err != nil {
				select {
				case ch.SendChan() <- utils.Result[*dns.Msg]{Error: err}:
				case <-ctx.Done():
				}
			} else {
				select {
				case ch.SendChan() <- utils.Result[*dns.Msg]{Value: resp}:
				case <-ctx.Done():
				}
			}
		}(uu, ch.Clone())
	}
	var lastErr error
	for i := 0; i < len(u.upstreams); i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-ch.ReceiveChan():
			if result.Error != nil {
				lastErr = result.Error
				continue
			}
			return result.Value, nil
		}
	}
	return nil, lastErr
}

func (u *ParallelUpstream) Exchange(ctx context.Context, req *dns.Msg) (resp *dns.Msg, err error) {
	resp, err = u.exchange(ctx, req)
	u.reqTotal.Add(1)
	if err == nil {
		u.reqSuccess.Add(1)
	}
	return
}

func (u *ParallelUpstream) StatisticalData() map[string]any {
	total := u.reqTotal.Load()
	success := u.reqSuccess.Load()
	return map[string]any{
		"total":   total,
		"success": success,
	}
}
