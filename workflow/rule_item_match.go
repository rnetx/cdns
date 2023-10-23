package workflow

import (
	"context"
	"fmt"
	"net/netip"
	"os"

	"github.com/miekg/dns"
	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"
	"gopkg.in/yaml.v3"
)

type RuleItemMatch struct {
	listener   []string
	clientIP   []netip.Prefix
	qType      []uint16
	qName      []string
	hasRespMsg *bool
	respIP     []netip.Prefix
	mark       []uint64
	env        map[string]string
	metadata   map[string]string
	plugin     *PluginMatcher
	//
	matchOr  []RuleItemMatch
	matchAnd []RuleItemMatch
	//
	invert bool
}

type _RuleItemMatch struct {
	Listener   utils.Listable[string] `yaml:"listener,omitempty"`
	ClientIP   utils.Listable[string] `yaml:"client-ip,omitempty"`
	QType      utils.Listable[any]    `yaml:"qtype,omitempty"`
	QName      utils.Listable[string] `yaml:"qname,omitempty"`
	HasRespMsg *bool                  `yaml:"has-resp-msg,omitempty"`
	RespIP     utils.Listable[string] `yaml:"resp-ip,omitempty"`
	Mark       utils.Listable[uint64] `yaml:"mark,omitempty"`
	Env        map[string]string      `yaml:"env,omitempty"`
	Metadata   map[string]string      `yaml:"metadata,omitempty"`
	Plugin     yaml.Node              `yaml:"plugin,omitempty"`
	//
	MatchOr  utils.Listable[yaml.Node] `yaml:"match-or,omitempty"`
	MatchAnd utils.Listable[yaml.Node] `yaml:"match-and,omitempty"`
	//
	Invert bool `yaml:"invert,omitempty"`
}

func (r *RuleItemMatch) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var _r _RuleItemMatch
	err := unmarshal(&_r)
	if err != nil {
		return err
	}
	tag := 0
	if len(_r.Listener) > 0 {
		r.listener = _r.Listener
		tag++
	}
	if len(_r.ClientIP) > 0 {
		r.clientIP = make([]netip.Prefix, 0, len(_r.ClientIP))
		for _, s := range _r.ClientIP {
			prefix, err := netip.ParsePrefix(s)
			if err == nil {
				r.clientIP = append(r.clientIP, prefix)
				continue
			}
			ip, err := netip.ParseAddr(s)
			if err == nil {
				bits := 0
				if ip.Is4() {
					bits = 32
				} else {
					bits = 128
				}
				r.clientIP = append(r.clientIP, netip.PrefixFrom(ip, bits))
				continue
			}
			return fmt.Errorf("invalid client-ip: %s", s)
		}
		tag++
	}
	if len(_r.QName) > 0 {
		r.qName = make([]string, 0, len(_r.QName))
		for _, v := range _r.QName {
			r.qName = append(r.qName, dns.Fqdn(v))
		}
		tag++
	}
	if len(_r.QType) > 0 {
		r.qType = make([]uint16, 0, len(_r.QType))
		for _, v := range _r.QType {
			switch vv := v.(type) {
			case string:
				t, ok := dns.StringToType[vv]
				if !ok {
					return fmt.Errorf("invalid qtype: %s", vv)
				}
				r.qType = append(r.qType, t)
			case int:
				r.qType = append(r.qType, uint16(vv))
			default:
				return fmt.Errorf("invalid qtype: %v", v)
			}
		}
		tag++
	}
	if _r.HasRespMsg != nil {
		r.hasRespMsg = new(bool)
		*r.hasRespMsg = *_r.HasRespMsg
		tag++
	}
	if len(_r.RespIP) > 0 {
		r.respIP = make([]netip.Prefix, 0, len(_r.RespIP))
		for _, s := range _r.RespIP {
			prefix, err := netip.ParsePrefix(s)
			if err == nil {
				r.respIP = append(r.respIP, prefix)
				continue
			}
			ip, err := netip.ParseAddr(s)
			if err == nil {
				bits := 0
				if ip.Is4() {
					bits = 32
				} else {
					bits = 128
				}
				r.respIP = append(r.respIP, netip.PrefixFrom(ip, bits))
				continue
			}
			return fmt.Errorf("invalid resp-ip: %s", s)
		}
		tag++
	}
	if len(_r.Mark) > 0 {
		r.mark = _r.Mark
		tag++
	}
	if _r.Env != nil && len(_r.Env) > 0 {
		r.env = _r.Env
		tag++
	}
	if _r.Metadata != nil && len(_r.Metadata) > 0 {
		r.metadata = _r.Metadata
		tag++
	}
	if !_r.Plugin.IsZero() {
		r.plugin = &PluginMatcher{}
		err := _r.Plugin.Decode(&r.plugin)
		if err != nil {
			return fmt.Errorf("invalid plugin: %v", err)
		}
		tag++
	}
	if len(_r.MatchOr) > 0 {
		r.matchOr = make([]RuleItemMatch, 0, len(_r.MatchOr))
		for _, v := range _r.MatchOr {
			var m RuleItemMatch
			err := v.Decode(&m)
			if err != nil {
				return fmt.Errorf("invalid match-or: %v", err)
			}
			r.matchOr = append(r.matchOr, m)
		}
		tag++
	}
	if len(_r.MatchAnd) > 0 {
		r.matchAnd = make([]RuleItemMatch, 0, len(_r.MatchAnd))
		for _, v := range _r.MatchAnd {
			var m RuleItemMatch
			err := v.Decode(&m)
			if err != nil {
				return fmt.Errorf("invalid match-and: %v", err)
			}
			r.matchAnd = append(r.matchAnd, m)
		}
		tag++
	}
	if tag != 1 {
		return fmt.Errorf("invalid match rule")
	}
	r.invert = _r.Invert
	return nil
}

func (r *RuleItemMatch) check(ctx context.Context, core adapter.Core) error {
	if len(r.listener) > 0 {
		for _, l := range r.listener {
			if core.GetListener(l) == nil {
				return fmt.Errorf("listener [%s] not found", l)
			}
		}
	}
	if r.plugin != nil {
		p := core.GetPluginMatcher(r.plugin.tag)
		if p == nil {
			return fmt.Errorf("plugin matcher [%s] not found", r.plugin.tag)
		}
		id := utils.RandomIDUint64()
		r.plugin.argsID = id
		err := p.LoadRunningArgs(ctx, r.plugin.argsID, r.plugin.args)
		if err != nil {
			return fmt.Errorf("plugin matcher [%s] load running args failed: %v", r.plugin.tag, err)
		}
		r.plugin.matcher = p
		r.plugin.tag = ""   // clean
		r.plugin.args = nil // clean
	}
	if len(r.matchOr) > 0 {
		for i, m := range r.matchOr {
			err := m.check(ctx, core)
			if err != nil {
				return fmt.Errorf("match-or[%d] check failed: %v", i, err)
			}
		}
	}
	if len(r.matchAnd) > 0 {
		for i, m := range r.matchAnd {
			err := m.check(ctx, core)
			if err != nil {
				return fmt.Errorf("match-and[%d] check failed: %v", i, err)
			}
		}
	}
	return nil
}

func (r *RuleItemMatch) match0(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	if len(r.listener) > 0 {
		ll := dnsCtx.Listener()
		for _, l := range r.listener {
			if ll == l {
				logger.DebugfContext(ctx, "match listener: %s", l)
				return true, nil
			}
		}
		logger.DebugfContext(ctx, "no match listener: %s", ll)
		return false, nil
	}
	if len(r.clientIP) > 0 {
		clientIP := dnsCtx.ClientIP()
		for _, prefix := range r.clientIP {
			if prefix.Contains(clientIP) {
				logger.DebugfContext(ctx, "match client-ip: %s => %s", prefix.String(), clientIP.String())
				return true, nil
			}
		}
		logger.DebugfContext(ctx, "no match client-ip: %s", clientIP.String())
		return false, nil
	}
	if len(r.qType) > 0 {
		question := dnsCtx.ReqMsg().Question
		if len(question) == 0 {
			logger.DebugfContext(ctx, "no match qtype: no question found")
			return false, nil
		}
		qType := question[0].Qtype
		for _, t := range r.qType {
			if t == qType {
				logger.DebugfContext(ctx, "match qtype: %s", dns.TypeToString[t])
				return true, nil
			}
		}
		logger.DebugfContext(ctx, "no match qtype: %s", dns.TypeToString[qType])
		return false, nil
	}
	if len(r.qName) > 0 {
		question := dnsCtx.ReqMsg().Question
		if len(question) == 0 {
			logger.DebugfContext(ctx, "no match qname: no question found")
			return false, nil
		}
		qName := question[0].Name
		for _, n := range r.qName {
			if n == qName {
				logger.DebugfContext(ctx, "match qname: %s", qName)
				return true, nil
			}
		}
		logger.DebugfContext(ctx, "no match qname: %s", qName)
		return false, nil
	}
	if r.hasRespMsg != nil {
		respMsg := dnsCtx.RespMsg()
		if *r.hasRespMsg && respMsg != nil {
			logger.DebugfContext(ctx, "match has-resp-msg: true")
			return true, nil
		}
		if !*r.hasRespMsg && respMsg == nil {
			logger.DebugfContext(ctx, "match has-resp-msg: false")
			return true, nil
		}
		logger.DebugfContext(ctx, "no match has-resp-msg: %t", respMsg != nil)
		return false, nil
	}
	if len(r.respIP) > 0 {
		answer := dnsCtx.RespMsg().Answer
		ips := make([]netip.Addr, 0, len(answer))
		for _, rr := range answer {
			switch a := rr.(type) {
			case *dns.A:
				ip, ok := netip.AddrFromSlice(a.A)
				if ok {
					ips = append(ips, ip)
				}
			case *dns.AAAA:
				ip, ok := netip.AddrFromSlice(a.AAAA)
				if ok {
					ips = append(ips, ip)
				}
			}
		}
		if len(ips) == 0 {
			logger.DebugfContext(ctx, "no match resp-ip: no ips found")
			return false, nil
		}
		for _, ip := range ips {
			for _, p := range r.respIP {
				if p.Contains(ip) {
					logger.DebugfContext(ctx, "match resp-ip: %s => %s", p.String(), ip.String())
					return true, nil
				}
			}
		}
		logger.DebugfContext(ctx, "no match resp-ip: %s", utils.Join(ips, ", "))
		return false, nil
	}
	if len(r.mark) > 0 {
		mark := dnsCtx.Mark()
		for _, m := range r.mark {
			if m == mark {
				logger.DebugfContext(ctx, "match mark: %d", m)
				return true, nil
			}
		}
		logger.DebugfContext(ctx, "no match mark: %d", mark)
		return false, nil
	}
	if len(r.env) > 0 {
		for k, v := range r.env {
			if os.Getenv(k) == v {
				logger.DebugfContext(ctx, "match env: %s => %s", k, v)
				return true, nil
			}
		}
		logger.DebugContext(ctx, "no match env")
		return false, nil
	}
	if len(r.metadata) > 0 {
		metadata := dnsCtx.Metadata()
		match := true
		for k1, v1 := range r.metadata {
			v2, ok := metadata[k1]
			if !ok || v2 != v1 {
				logger.DebugfContext(ctx, "no match metadata: %s => %s, value: %s", k1, v1, v2)
				match = false
				break
			}
		}
		if match {
			logger.DebugContext(ctx, "match metadata")
			return true, nil
		} else {
			logger.DebugContext(ctx, "no match metadata")
			return false, nil
		}
	}
	if r.plugin != nil {
		matched, err := r.plugin.matcher.Match(ctx, dnsCtx, r.plugin.argsID)
		if err != nil {
			logger.DebugfContext(ctx, "plugin matcher [%s] match failed: %v", r.plugin.matcher.Tag(), err)
			return false, err
		}
		logger.DebugfContext(ctx, "plugin matcher [%s] match: %t", r.plugin.matcher.Tag(), matched)
		return matched, nil
	}
	if len(r.matchOr) > 0 {
		match := false
		for i, m := range r.matchOr {
			logger.DebugfContext(ctx, "match match-or[%d]", i)
			matched, err := m.match(ctx, core, logger, dnsCtx)
			if err != nil {
				logger.DebugfContext(ctx, "match match-or[%d] failed: %v", i, err)
				return false, err
			}
			if matched {
				logger.DebugfContext(ctx, "match match-or[%d] => true", i)
				match = true
				break
			}
			logger.DebugfContext(ctx, "match match-or[%d] => false, continue", i)
		}
		if match {
			logger.DebugfContext(ctx, "match match-or: true")
			return true, nil
		} else {
			logger.DebugfContext(ctx, "no match match-or")
			return false, nil
		}
	}
	if len(r.matchAnd) > 0 {
		match := true
		for i, m := range r.matchAnd {
			logger.DebugfContext(ctx, "match match-and[%d]", i)
			matched, err := m.match(ctx, core, logger, dnsCtx)
			if err != nil {
				logger.DebugfContext(ctx, "match match-and[%d] failed: %v", i, err)
				return false, err
			}
			if !matched {
				logger.DebugfContext(ctx, "match match-and[%d] => false", i)
				match = false
				break
			}
			logger.DebugfContext(ctx, "match match-and[%d] => true, continue", i)
		}
		if match {
			logger.DebugfContext(ctx, "match match-and: true")
			return true, nil
		} else {
			logger.DebugfContext(ctx, "no match match-and")
			return false, nil
		}
	}
	panic("unreachable")
}

func (r *RuleItemMatch) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	matched, err := r.match0(ctx, core, logger, dnsCtx)
	if err != nil {
		return matched, err
	}
	if r.invert {
		logger.DebugfContext(ctx, "invert match: %t => %t", matched, !matched)
		return !matched, nil
	}
	return matched, nil
}
