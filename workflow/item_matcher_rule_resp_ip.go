package workflow

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
)

var _ itemMatcherRule = (*itemMatcherRespIPRule)(nil)

type itemMatcherRespIPRule struct {
	respIP []netip.Prefix
}

func (r *itemMatcherRespIPRule) UnmarshalYAML(value *yaml.Node) error {
	var rr utils.Listable[string]
	err := value.Decode(&rr)
	if err != nil {
		return fmt.Errorf("resp-ip: %w", err)
	}
	if len(rr) == 0 {
		return fmt.Errorf("resp-ip: missing resp-ip")
	}
	r.respIP = make([]netip.Prefix, 0, len(rr))
	for _, s := range rr {
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
		return fmt.Errorf("resp-ip: invalid resp-ip: %s", s)
	}
	return nil
}

func (r *itemMatcherRespIPRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemMatcherRespIPRule) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
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
		logger.DebugfContext(ctx, "resp-ip: no match resp-ip: no ips found")
		return false, nil
	}
	for _, ip := range ips {
		for _, p := range r.respIP {
			if p.Contains(ip) {
				logger.DebugfContext(ctx, "resp-ip: match resp-ip: %s => %s", p.String(), ip.String())
				return true, nil
			}
		}
	}
	logger.DebugfContext(ctx, "resp-ip: no match resp-ip: %s", utils.Join(ips, ", "))
	return false, nil
}
