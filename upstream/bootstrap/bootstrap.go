package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
)

const DefaultCacheTTL = 300 * time.Second

const (
	StrategyIPv4Prefer = "ipv4-prefer"
	StrategyIPv6Prefer = "ipv6-prefer"
	StrategyIPv4Only   = "ipv4-only"
	StrategyIPv6Only   = "ipv6-only"
)

type Options struct {
	Upstream string         `yaml:"upstream"`
	Strategy string         `yaml:"strategy,omitempty"`
	CacheTTL utils.Duration `yaml:"cache-ttl,omitempty"`
}

type Bootstrap struct {
	ctx         context.Context
	core        adapter.Core
	upstreamTag string
	upstream    adapter.Upstream
	strategy    string
	cacheTTL    time.Duration

	loopCtx    context.Context
	loopCancel context.CancelFunc
	closeDone  chan struct{}
	isClosed   bool
	loopDone   chan struct{}
	cacheLock  sync.Mutex
	cacheTime  time.Time
	cache      []netip.Addr
}

func NewBootstrap(ctx context.Context, core adapter.Core, options Options) (*Bootstrap, error) {
	b := &Bootstrap{
		ctx:       ctx,
		closeDone: make(chan struct{}, 1),
		core:      core,
		cacheTTL:  time.Duration(options.CacheTTL),
	}
	if options.Upstream == "" {
		return nil, errors.New("missing upstream")
	}
	b.upstreamTag = options.Upstream
	switch options.Strategy {
	case StrategyIPv4Prefer, "":
		b.strategy = StrategyIPv4Prefer
	case StrategyIPv6Prefer:
		b.strategy = StrategyIPv6Prefer
	case StrategyIPv4Only:
		b.strategy = StrategyIPv4Only
	case StrategyIPv6Only:
		b.strategy = StrategyIPv6Only
	default:
		return nil, fmt.Errorf("invalid strategy: %s", options.Strategy)
	}
	if b.cacheTTL <= 0 {
		b.cacheTTL = DefaultCacheTTL
	}
	return b, nil
}

func (b *Bootstrap) Start() error {
	u := b.core.GetUpstream(b.upstreamTag)
	if u == nil {
		return fmt.Errorf("upstream [%s] not found", b.upstreamTag)
	}
	b.upstream = u
	b.loopCtx, b.loopCancel = context.WithCancel(b.ctx)
	go b.loopHandle()
	return nil
}

func (b *Bootstrap) Close() {
	if b.isClosed {
		return
	}
	b.isClosed = true
	b.loopCancel()
	<-b.closeDone
	close(b.closeDone)
}

func (b *Bootstrap) loopHandle() {
	defer func() {
		select {
		case b.closeDone <- struct{}{}:
		default:
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-b.loopCtx.Done():
			return
		case <-ticker.C:
			b.cacheLock.Lock()
			if !b.cacheTime.IsZero() && time.Since(b.cacheTime) > b.cacheTTL {
				b.cache = nil
				b.cacheTime = time.Time{}
			}
			b.cacheLock.Unlock()
		}
	}
}

func (b *Bootstrap) UpstreamTag() string {
	return b.upstreamTag
}

func (b *Bootstrap) lookup0(ctx context.Context, req *dns.Msg) ([]netip.Addr, error) {
	resp, err := b.upstream.Exchange(ctx, req)
	if err != nil {
		return nil, err
	}
	var ips []netip.Addr
	for _, answer := range resp.Answer {
		switch rr := answer.(type) {
		case *dns.A:
			ip, ok := netip.AddrFromSlice(rr.A)
			if ok {
				ips = append(ips, ip)
			}
		case *dns.AAAA:
			ip, ok := netip.AddrFromSlice(rr.AAAA)
			if ok {
				ips = append(ips, ip)
			}
		}
	}
	if len(ips) == 0 {
		return nil, errors.New("no ip found")
	}
	return ips, nil
}

func newDNSMessage(domain string, recordType uint16) *dns.Msg {
	req := &dns.Msg{}
	req.SetQuestion(dns.Fqdn(domain), recordType)
	return req
}

type lookupResult struct {
	ips    []netip.Addr
	isAAAA bool
}

func (b *Bootstrap) lookup(ctx context.Context, domain string) ([]netip.Addr, error) {
	if b.strategy == StrategyIPv4Only {
		return b.lookup0(ctx, newDNSMessage(domain, dns.TypeA))
	}
	if b.strategy == StrategyIPv6Only {
		return b.lookup0(ctx, newDNSMessage(domain, dns.TypeAAAA))
	}
	ch := utils.NewSafeChan[utils.Result[lookupResult]](2)
	defer ch.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	reqMsgA := newDNSMessage(domain, dns.TypeA)
	reqMsgAAAA := newDNSMessage(domain, dns.TypeAAAA)
	go func(ch *utils.SafeChan[utils.Result[lookupResult]]) {
		defer ch.Close()
		ips, err := b.lookup0(ctx, reqMsgA)
		if err != nil {
			select {
			case ch.SendChan() <- utils.Result[lookupResult]{Error: err}:
			case <-ctx.Done():
			}
		} else {
			select {
			case ch.SendChan() <- utils.Result[lookupResult]{Value: lookupResult{ips: ips}}:
			case <-ctx.Done():
			}
		}
	}(ch.Clone())
	go func(ch *utils.SafeChan[utils.Result[lookupResult]]) {
		defer ch.Close()
		ips, err := b.lookup0(ctx, reqMsgAAAA)
		if err != nil {
			select {
			case ch.SendChan() <- utils.Result[lookupResult]{Error: err}:
			case <-ctx.Done():
			}
		} else {
			select {
			case ch.SendChan() <- utils.Result[lookupResult]{Value: lookupResult{ips: ips, isAAAA: true}}:
			case <-ctx.Done():
			}
		}
	}(ch.Clone())
	var (
		lastIPs []netip.Addr
		lastErr error
	)
	for i := 0; i < 2; i++ {
		select {
		case result := <-ch.ReceiveChan():
			if result.Error != nil {
				lastErr = result.Error
				continue
			}
			if b.strategy == StrategyIPv4Prefer && !result.Value.isAAAA {
				if len(lastIPs) > 0 {
					result.Value.ips = append(result.Value.ips, lastIPs...)
				}
				return result.Value.ips, nil
			}
			if b.strategy == StrategyIPv6Prefer && result.Value.isAAAA {
				if len(lastIPs) > 0 {
					result.Value.ips = append(result.Value.ips, lastIPs...)
				}
				return result.Value.ips, nil
			}
			lastIPs = result.Value.ips
			continue
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return lastIPs, nil
}

func (b *Bootstrap) Lookup(ctx context.Context, domain string) ([]netip.Addr, error) {
	cacheTime := b.cacheTime
	if !cacheTime.IsZero() && time.Since(cacheTime) < b.cacheTTL {
		return b.cache, nil
	}
	b.cacheLock.Lock()
	defer b.cacheLock.Unlock()
	if !b.cacheTime.IsZero() && time.Since(b.cacheTime) < b.cacheTTL {
		return b.cache, nil
	}
	ips, err := b.lookup(ctx, domain)
	if err != nil {
		return nil, err
	}
	b.cache = ips
	b.cacheTime = time.Now()
	return ips, nil
}
