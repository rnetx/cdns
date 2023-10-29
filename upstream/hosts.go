package upstream

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"sync/atomic"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/dlclark/regexp2"
	"github.com/miekg/dns"
)

type HostsUpstreamOptions struct {
	Rule     map[string]utils.Listable[string] `yaml:"rule"`
	Fallback string                            `yaml:"fallback"`
}

const HostsUpstreamType = "hosts"

var (
	_ adapter.Upstream = (*HostsUpstream)(nil)
	_ adapter.Starter  = (*HostsUpstream)(nil)
)

type HostsUpstream struct {
	ctx    context.Context
	tag    string
	core   adapter.Core
	logger log.Logger

	rule        []*hostsRule
	fallbackTag string
	fallback    adapter.Upstream

	reqTotal   atomic.Uint64
	reqSuccess atomic.Uint64
}

type hostsRule struct {
	rule *regexp2.Regexp
	ipv4 bool
	ipv6 bool
	ip   []netip.Prefix
}

func NewHostsUpstream(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options HostsUpstreamOptions) (adapter.Upstream, error) {
	u := &HostsUpstream{
		ctx:    ctx,
		tag:    tag,
		core:   core,
		logger: logger,
	}
	if len(options.Rule) == 0 {
		return nil, fmt.Errorf("create hosts upstream failed: missing rule")
	}
	rule := make([]*hostsRule, 0, len(options.Rule))
	for k, v := range options.Rule {
		r, err := regexp2.Compile(k, regexp2.RE2)
		if err != nil {
			return nil, fmt.Errorf("create hosts upstream failed: invalid rule: %s, error: %w", k, err)
		}
		if len(v) == 0 {
			return nil, fmt.Errorf("create hosts upstream failed: missing ip")
		}
		var (
			ipv4 bool
			ipv6 bool
		)
		ips := make([]netip.Prefix, 0, len(v))
		for _, s := range v {
			prefix, err := netip.ParsePrefix(s)
			if err == nil {
				ip := prefix.Addr()
				if ip.Is4() {
					ipv4 = true
				} else {
					ipv6 = true
				}
				ips = append(ips, prefix)
				continue
			}
			ip, err := netip.ParseAddr(s)
			if err == nil {
				bits := 0
				if ip.Is4() {
					bits = 32
					ipv4 = true
				} else {
					bits = 128
					ipv6 = true
				}
				ips = append(ips, netip.PrefixFrom(ip, bits))
				continue
			}
			return nil, fmt.Errorf("create hosts upstream failed: invalid ip: %s, error: %w", s, err)
		}
		rule = append(rule, &hostsRule{
			rule: r,
			ipv4: ipv4,
			ipv6: ipv6,
			ip:   ips,
		})
	}
	u.rule = rule
	if options.Fallback == "" {
		return nil, fmt.Errorf("create hosts upstream failed: missing fallback")
	}
	u.fallbackTag = options.Fallback
	return u, nil
}

func (u *HostsUpstream) Tag() string {
	return u.tag
}

func (u *HostsUpstream) Type() string {
	return HostsUpstreamType
}

func (u *HostsUpstream) Start() error {
	uu := u.core.GetUpstream(u.fallbackTag)
	if uu == nil {
		return fmt.Errorf("upstream [%s] not found", u.fallbackTag)
	}
	u.fallback = uu
	return nil
}

func (u *HostsUpstream) Dependencies() []string {
	return []string{u.fallbackTag}
}

func (u *HostsUpstream) exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	question := req.Question[0]
	qName := question.Name
	qName = strings.TrimSuffix(qName, ".")
	qType := question.Qtype
	for _, r := range u.rule {
		matched, err := r.rule.MatchString(qName)
		if err == nil && matched {
			if (qType == dns.TypeA && r.ipv4) || (qType == dns.TypeAAAA && r.ipv6) {
				answers := make([]dns.RR, 0, len(r.ip))
				for _, p := range r.ip {
					var record dns.RR
					ip := p.Addr()
					if qType == dns.TypeA && ip.Is4() {
						if p.Bits() != 32 {
							ip = utils.RandomAddrFromPrefix(p)
						}
						record = &dns.A{
							Hdr: dns.RR_Header{
								Name:   qName,
								Rrtype: dns.TypeA,
								Class:  dns.ClassINET,
								Ttl:    600,
							},
							A: ip.AsSlice(),
						}
					}
					if qType == dns.TypeAAAA && ip.Is6() {
						if p.Bits() != 128 {
							ip = utils.RandomAddrFromPrefix(p)
						}
						record = &dns.AAAA{
							Hdr: dns.RR_Header{
								Name:   qName,
								Rrtype: dns.TypeAAAA,
								Class:  dns.ClassINET,
								Ttl:    600,
							},
							AAAA: ip.AsSlice(),
						}
					}
					answers = append(answers, record)
				}
				respMsg := &dns.Msg{}
				respMsg.SetReply(req)
				respMsg.Answer = answers
				return respMsg, nil
			}
		}
	}
	return u.fallback.Exchange(ctx, req)
}

func (u *HostsUpstream) Exchange(ctx context.Context, req *dns.Msg) (resp *dns.Msg, err error) {
	u.reqTotal.Add(1)
	resp, err = u.exchange(ctx, req)
	if err == nil {
		u.reqSuccess.Add(1)
	}
	return
}

func (u *HostsUpstream) StatisticalData() map[string]any {
	total := u.reqTotal.Load()
	success := u.reqSuccess.Load()
	return map[string]any{
		"total":   total,
		"success": success,
	}
}
