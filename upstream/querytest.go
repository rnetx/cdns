package upstream

import (
	"context"
	"fmt"
	"time"

	"github.com/miekg/dns"
	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"
)

const (
	DefaultQueryTestDomain    = "www.example.com"
	DefaultQueryTestInterval  = 10 * time.Minute
	DefaultQueryTestTolerance = 3 * time.Millisecond
)

type QueryTestUpstreamOptions struct {
	Upstreams    []string       `yaml:"upstreams"`
	TestDomain   string         `yaml:"test-domain,omitempty"`
	TestInterval utils.Duration `yaml:"test-interval,omitempty"`
	Tolerance    utils.Duration `yaml:"tolerance,omitempty"`
}

const QueryTestUpstreamType = "querytest"

var (
	_ adapter.Upstream = (*QueryTestUpstream)(nil)
	_ adapter.Starter  = (*QueryTestUpstream)(nil)
	_ adapter.Closer   = (*QueryTestUpstream)(nil)
)

type QueryTestUpstream struct {
	ctx    context.Context
	tag    string
	core   adapter.Core
	logger log.Logger

	upstreamTags []string
	upstreams    []adapter.Upstream
	selected     adapter.Upstream
	selectedTest time.Duration

	testDomain   string
	testInterval time.Duration
	tolerance    time.Duration

	loopCtx    context.Context
	loopCancel context.CancelFunc
	closeDone  chan struct{}
}

func NewQueryTestUpstream(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options QueryTestUpstreamOptions) (adapter.Upstream, error) {
	u := &QueryTestUpstream{
		ctx:          ctx,
		tag:          tag,
		core:         core,
		logger:       logger,
		upstreamTags: options.Upstreams,
	}
	if len(u.upstreamTags) == 0 {
		return nil, fmt.Errorf("create querytest upstream failed: missing upstreams")
	}
	if options.TestDomain != "" {
		u.testDomain = options.TestDomain
	} else {
		u.testDomain = DefaultQueryTestDomain
	}
	if options.TestInterval > 0 {
		u.testInterval = time.Duration(options.TestInterval)
	} else {
		u.testInterval = DefaultQueryTestInterval
	}
	if options.Tolerance > 0 {
		u.tolerance = time.Duration(options.Tolerance)
	} else {
		u.tolerance = DefaultQueryTestTolerance
	}
	return u, nil
}

func (u *QueryTestUpstream) Tag() string {
	return u.tag
}

func (u *QueryTestUpstream) Type() string {
	return QueryTestUpstreamType
}

func (u *QueryTestUpstream) Dependencies() []string {
	return u.upstreamTags
}

func (u *QueryTestUpstream) Start() error {
	u.upstreams = make([]adapter.Upstream, 0, len(u.upstreamTags))
	for _, tag := range u.upstreamTags {
		uu := u.core.GetUpstream(tag)
		if uu == nil {
			return fmt.Errorf("upstream [%s] not found", tag)
		}
		u.upstreams = append(u.upstreams, uu)
	}
	u.selected = u.upstreams[0]
	u.loopCtx, u.loopCancel = context.WithCancel(u.ctx)
	u.closeDone = make(chan struct{}, 1)
	go u.loopHandle()
	return nil
}

func (u *QueryTestUpstream) Close() error {
	u.loopCancel()
	<-u.closeDone
	close(u.closeDone)
	return nil
}

func (u *QueryTestUpstream) loopHandle() {
	u.test(u.ctx)
	select {
	case <-u.ctx.Done():
		return
	default:
	}
	defer func() {
		select {
		case u.closeDone <- struct{}{}:
		default:
		}
	}()
	ticker := time.NewTicker(u.testInterval)
	defer ticker.Stop()
	for {
		select {
		case <-u.loopCtx.Done():
			return
		case <-ticker.C:
			u.test(u.loopCtx)
		}
	}
}

func (u *QueryTestUpstream) newDNSMsg() *dns.Msg {
	msg := &dns.Msg{}
	msg.SetQuestion(dns.Fqdn(u.testDomain), dns.TypeA)
	return msg
}

type testResult struct {
	uu       adapter.Upstream
	duration time.Duration
}

func (u *QueryTestUpstream) test(ctx context.Context) {
	u.logger.Debug("run test...")
	defer u.logger.Debug("test done")
	msg := u.newDNSMsg()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := utils.NewSafeChan[utils.Result[testResult]](len(u.upstreams))
	defer ch.Close()
	for _, uu := range u.upstreams {
		go func(uu adapter.Upstream, req *dns.Msg, ch *utils.SafeChan[utils.Result[testResult]]) {
			defer ch.Close()
			req.Id = dns.Id()
			t := time.Now()
			_, err := uu.Exchange(ctx, req)
			duration := time.Since(t)
			if err != nil {
				select {
				case ch.SendChan() <- utils.Result[testResult]{Error: err}:
				case <-ctx.Done():
				}
			} else {
				select {
				case ch.SendChan() <- utils.Result[testResult]{Value: testResult{uu: uu, duration: duration}}:
				case <-ctx.Done():
				}
			}
		}(uu, msg.Copy(), ch.Clone())
	}
	selected := u.selected
	selectedTest := u.selectedTest
	for i := 0; i < len(u.upstreams); i++ {
		select {
		case <-ctx.Done():
			return
		case result := <-ch.ReceiveChan():
			if result.Error != nil {
				continue
			}
			if selectedTest == 0 || result.Value.duration < selectedTest-u.tolerance {
				selected = result.Value.uu
				selectedTest = result.Value.duration
			}
		}
	}
	u.selected = selected
	u.selectedTest = selectedTest
}

func (u *QueryTestUpstream) Exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	selected := u.selected
	u.logger.DebugfContext(ctx, "selected upstream: %s", selected.Tag())
	return selected.Exchange(ctx, req)
}
