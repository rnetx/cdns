package upstream

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
)

const (
	DefaultFallbackDomain   = "www.example.com"
	DefaultFallbackInterval = 10 * time.Minute
)

type FallbackUpstreamOptions struct {
	MainUpstream     string         `yaml:"main-upstream"`
	FallbackUpstream string         `yaml:"fallback-upstream"`
	TestDomain       string         `yaml:"test-domain,omitempty"`
	TestInterval     utils.Duration `yaml:"test-interval,omitempty"`
}

const FallbackUpstreamType = "fallback"

type FallbackUpstream struct {
	ctx    context.Context
	tag    string
	core   adapter.Core
	logger log.Logger

	mainUpstreamTag     string
	mainUpstream        adapter.Upstream
	fallbackUpstreamTag string
	fallbackUpstream    adapter.Upstream

	testDomain   string
	testInterval time.Duration
	healthy      bool
	callChan     chan struct{}
	loopCtx      context.Context
	loopCancel   context.CancelFunc
	closeDone    chan struct{}

	reqTotal   atomic.Uint64
	reqSuccess atomic.Uint64
}

func NewFallbackUpstream(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options FallbackUpstreamOptions) (adapter.Upstream, error) {
	u := &FallbackUpstream{
		ctx:                 ctx,
		tag:                 tag,
		core:                core,
		logger:              logger,
		mainUpstreamTag:     options.MainUpstream,
		fallbackUpstreamTag: options.FallbackUpstream,
	}
	if u.mainUpstreamTag == "" {
		return nil, fmt.Errorf("create fallback upstream failed: missing main-upstream")
	}
	if u.fallbackUpstreamTag == "" {
		return nil, fmt.Errorf("create fallback upstream failed: missing fallback-upstream")
	}
	if options.TestDomain != "" {
		u.testDomain = options.TestDomain
	} else {
		u.testDomain = DefaultFallbackDomain
	}
	if options.TestInterval > 0 {
		u.testInterval = time.Duration(options.TestInterval)
	} else {
		u.testInterval = DefaultFallbackInterval
	}
	return u, nil
}

func (u *FallbackUpstream) Tag() string {
	return u.tag
}

func (u *FallbackUpstream) Type() string {
	return FallbackUpstreamType
}

func (u *FallbackUpstream) Dependencies() []string {
	return []string{u.mainUpstreamTag, u.fallbackUpstreamTag}
}

func (u *FallbackUpstream) Start() error {
	u.mainUpstream = u.core.GetUpstream(u.mainUpstreamTag)
	if u.mainUpstream == nil {
		return fmt.Errorf("upstream [%s] not found", u.mainUpstreamTag)
	}
	u.fallbackUpstream = u.core.GetUpstream(u.fallbackUpstreamTag)
	if u.fallbackUpstream == nil {
		return fmt.Errorf("upstream [%s] not found", u.fallbackUpstreamTag)
	}
	u.healthy = true
	u.callChan = make(chan struct{}, 1)
	u.closeDone = make(chan struct{}, 1)
	u.loopCtx, u.loopCancel = context.WithCancel(u.ctx)
	go u.loopHandle()
	return nil
}

func (u *FallbackUpstream) Close() error {
	u.loopCancel()
	<-u.closeDone
	close(u.closeDone)
	close(u.callChan)
	return nil
}

func (u *FallbackUpstream) loopHandle() {
	defer func() {
		select {
		case u.closeDone <- struct{}{}:
		default:
		}
	}()
	for {
		select {
		case <-u.ctx.Done():
			return
		case <-u.callChan:
			if err := u.check(u.ctx); err == nil {
				continue
			}
			for {
				timer := time.NewTimer(u.testInterval)
				select {
				case <-u.ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
					timer.Stop()
					if err := u.check(u.ctx); err == nil {
						break
					}
					continue
				}
				break
			}
		}
	}
}

func (u *FallbackUpstream) newDNSMsg() *dns.Msg {
	msg := &dns.Msg{}
	msg.SetQuestion(dns.Fqdn(u.testDomain), dns.TypeA)
	return msg
}

func (u *FallbackUpstream) check(ctx context.Context) error {
	u.logger.Debugf("check main upstream [%s] ...", u.mainUpstreamTag)
	msg := u.newDNSMsg()
	_, err := u.mainUpstream.Exchange(ctx, msg)
	if err == nil {
		u.healthy = true
	}
	return err
}

func (u *FallbackUpstream) exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	if !u.healthy {
		return u.fallbackUpstream.Exchange(ctx, req)
	}
	resp, err := u.mainUpstream.Exchange(ctx, req)
	if err != nil {
		// TODO: DNS Error Or Network Error ??
		u.healthy = false
		select {
		case u.callChan <- struct{}{}:
			u.logger.Debugf("main upstream [%s] is unhealthy, fallback to fallback upstream [%s] and check main upstream", u.mainUpstreamTag, u.fallbackUpstreamTag)
		default:
		}
		return u.fallbackUpstream.Exchange(ctx, req)
	}
	return resp, nil
}

func (u *FallbackUpstream) Exchange(ctx context.Context, req *dns.Msg) (resp *dns.Msg, err error) {
	resp, err = u.exchange(ctx, req)
	u.reqTotal.Add(1)
	if err == nil {
		u.reqSuccess.Add(1)
	}
	return
}

func (u *FallbackUpstream) StatisticalData() map[string]any {
	total := u.reqTotal.Load()
	success := u.reqSuccess.Load()
	return map[string]any{
		"total":   total,
		"success": success,
	}
}
