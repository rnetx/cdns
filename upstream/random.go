package upstream

import (
	"context"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
)

type RandomUpstreamOptions struct {
	Upstreams []string `yaml:"upstreams"`
}

const RandomUpstreamType = "random"

var (
	_ adapter.Upstream = (*RandomUpstream)(nil)
	_ adapter.Starter  = (*RandomUpstream)(nil)
)

type RandomUpstream struct {
	tag    string
	core   adapter.Core
	logger log.Logger

	upstreamTags []string
	upstreams    []adapter.Upstream

	reqTotal   atomic.Uint64
	reqSuccess atomic.Uint64
}

func NewRandomUpstream(_ context.Context, core adapter.Core, logger log.Logger, tag string, options RandomUpstreamOptions) (adapter.Upstream, error) {
	u := &RandomUpstream{
		tag:          tag,
		core:         core,
		logger:       logger,
		upstreamTags: options.Upstreams,
	}
	if len(u.upstreamTags) == 0 {
		return nil, fmt.Errorf("create random upstream failed: missing upstreams")
	}
	return u, nil
}

func (u *RandomUpstream) Tag() string {
	return u.tag
}

func (u *RandomUpstream) Type() string {
	return RandomUpstreamType
}

func (u *RandomUpstream) Dependencies() []string {
	return u.upstreamTags
}

func (u *RandomUpstream) Start() error {
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

func (u *RandomUpstream) exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	index := r.Intn(len(u.upstreams))
	uu := u.upstreams[index]
	u.logger.DebugfContext(ctx, "random upstream [%s] selected", uu.Tag())
	return uu.Exchange(ctx, req)
}

func (u *RandomUpstream) Exchange(ctx context.Context, req *dns.Msg) (resp *dns.Msg, err error) {
	resp, err = u.exchange(ctx, req)
	u.reqTotal.Add(1)
	if err == nil {
		u.reqSuccess.Add(1)
	}
	return
}

func (u *RandomUpstream) StatisticalData() map[string]any {
	total := u.reqTotal.Load()
	success := u.reqSuccess.Load()
	return map[string]any{
		"total":   total,
		"success": success,
	}
}
