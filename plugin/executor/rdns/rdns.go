package rdns

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
)

const Type = "rdns"

func init() {
	plugin.RegisterPluginExecutor(Type, NewRDNS)
}

type rule struct {
	upstream adapter.Upstream
	rule     netip.Prefix
	isAny    bool
}

var _ adapter.PluginExecutor = (*RDNS)(nil)

type RDNS struct {
	ctx    context.Context
	core   adapter.Core
	tag    string
	logger log.Logger

	runningArgsMap map[uint16][]rule
}

func NewRDNS(ctx context.Context, core adapter.Core, logger log.Logger, tag string, _ any) (adapter.PluginExecutor, error) {
	r := &RDNS{
		ctx:    ctx,
		core:   core,
		tag:    tag,
		logger: logger,
	}
	return r, nil
}

func (r *RDNS) Tag() string {
	return r.tag
}

func (r *RDNS) Type() string {
	return Type
}

func (r *RDNS) Exec(ctx context.Context, dnsCtx *adapter.DNSContext, argsID uint16) (adapter.ReturnMode, error) {
	reqMsg := dnsCtx.ReqMsg()
	if reqMsg == nil {
		r.logger.DebugContext(ctx, "request message is nil")
		return adapter.ReturnModeContinue, nil
	}
	q := reqMsg.Question[0]
	ip := isIPv4rDNS(&q)
	if !ip.IsValid() {
		ip = isIPv6rDNS(&q)
		if !ip.IsValid() {
			r.logger.DebugContext(ctx, "not a reverse dns query")
			return adapter.ReturnModeContinue, nil
		}
	}
	rules := r.runningArgsMap[argsID]
	for _, rule := range rules {
		if rule.isAny || rule.rule.Contains(ip) {
			respMsg, err := rule.upstream.Exchange(ctx, reqMsg)
			if err != nil {
				return adapter.ReturnModeUnknown, err
			}
			dnsCtx.SetRespMsg(respMsg)
			return adapter.ReturnModeContinue, nil
		}
	}
	return adapter.ReturnModeContinue, nil
}

func (r *RDNS) LoadRunningArgs(ctx context.Context, args any) (uint16, error) {
	var rawRuleMap map[string]string
	err := utils.JsonDecode(args, &rawRuleMap)
	if err != nil {
		return 0, err
	}
	if len(rawRuleMap) == 0 {
		return 0, fmt.Errorf("missing rule")
	}
	rules := make([]rule, 0, len(rawRuleMap))
	for ruleStr, upstreamTag := range rawRuleMap {
		isAny := ruleStr == "*"
		var prefix netip.Prefix
		if !isAny {
			prefix, err = netip.ParsePrefix(ruleStr)
			if err != nil {
				ip, err2 := netip.ParseAddr(ruleStr)
				if err2 != nil {
					return 0, fmt.Errorf("parse rule failed: %w | %w", err, err2)
				}
				bits := 0
				if ip.Is6() {
					bits = 128
				} else {
					bits = 32
				}
				prefix = netip.PrefixFrom(ip, bits)
			}
		}
		u := r.core.GetUpstream(upstreamTag)
		if u == nil {
			return 0, fmt.Errorf("upstream [%s] not found", upstreamTag)
		}
		rules = append(rules, rule{
			upstream: u,
			rule:     prefix,
			isAny:    isAny,
		})
	}
	if r.runningArgsMap == nil {
		r.runningArgsMap = make(map[uint16][]rule)
	}
	var id uint16
	for {
		id = utils.RandomIDUint16()
		if _, ok := r.runningArgsMap[id]; !ok {
			break
		}
	}
	r.runningArgsMap[id] = rules
	return id, nil
}

func isIPv4rDNS(q *dns.Question) netip.Addr {
	if q.Qtype == dns.TypePTR && q.Qclass == dns.ClassINET && strings.HasSuffix(q.Name, ".in-addr.arpa.") {
		name := q.Name[:len(q.Name)-len(".in-addr.arpa.")]
		ips := strings.Split(name, ".")
		if len(ips) != 4 {
			return netip.Addr{}
		}
		ipStr := fmt.Sprintf("%s.%s.%s.%s", ips[3], ips[2], ips[1], ips[0])
		ip, err := netip.ParseAddr(ipStr)
		if err == nil {
			if ip.Is4() {
				return ip
			}
		}
	}
	return netip.Addr{}
}

func isIPv6rDNS(q *dns.Question) netip.Addr {
	if q.Qtype == dns.TypePTR && q.Qclass == dns.ClassINET && strings.HasSuffix(q.Name, ".ip6.arpa.") {
		name := q.Name[:len(q.Name)-len(".ip6.arpa.")]
		rawIPStr := ""
		s := 0
		m := 0
		for i := len(name) - 1; i >= 0; i-- {
			if s == 4 {
				rawIPStr += ":"
				m++
				s = 0
			}
			if name[i] != '.' {
				rawIPStr += string(name[i])
				s++
			}
		}
		if m != 7 {
			rawIPStr += "::"
		}
		ip, err := netip.ParseAddr(rawIPStr)
		if err == nil {
			if ip.Is6() {
				return ip
			}
		}
	}
	return netip.Addr{}
}
