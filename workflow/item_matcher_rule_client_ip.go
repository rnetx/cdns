package workflow

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"gopkg.in/yaml.v3"
)

var _ itemMatcherRule = (*itemMatcherClientIPRule)(nil)

type itemMatcherClientIPRule struct {
	clientIP []netip.Prefix
}

func (r *itemMatcherClientIPRule) UnmarshalYAML(value *yaml.Node) error {
	var c utils.Listable[string]
	err := value.Decode(&c)
	if err != nil {
		return fmt.Errorf("client-ip: %w", err)
	}
	if len(c) == 0 {
		return fmt.Errorf("client-ip: missing client-ip")
	}
	r.clientIP = make([]netip.Prefix, 0, len(c))
	for _, s := range c {
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
		return fmt.Errorf("client-ip: invalid client-ip: %s", s)
	}
	return nil
}

func (r *itemMatcherClientIPRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemMatcherClientIPRule) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	clientIP := dnsCtx.ClientIP()
	for _, prefix := range r.clientIP {
		if prefix.Contains(clientIP) {
			logger.DebugfContext(ctx, "client-ip: match client-ip: %s => %s", prefix.String(), clientIP.String())
			return true, nil
		}
	}
	logger.DebugfContext(ctx, "client-ip: no match client-ip: %s", clientIP.String())
	return false, nil
}
