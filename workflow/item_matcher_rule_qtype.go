package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
)

var _ itemMatcherRule = (*itemMatcherQTypeRule)(nil)

type itemMatcherQTypeRule struct {
	qType []uint16
}

func (r *itemMatcherQTypeRule) UnmarshalYAML(value *yaml.Node) error {
	var q utils.Listable[any]
	err := value.Decode(&q)
	if err != nil {
		return fmt.Errorf("qtype: %w", err)
	}
	if len(q) == 0 {
		return fmt.Errorf("qtype: missing qtype")
	}
	r.qType = make([]uint16, 0, len(r.qType))
	for _, v := range q {
		switch vv := v.(type) {
		case string:
			t, ok := dns.StringToType[vv]
			if !ok {
				return fmt.Errorf("qtype: invalid qtype: %s", vv)
			}
			r.qType = append(r.qType, t)
		case int:
			r.qType = append(r.qType, uint16(vv))
		default:
			return fmt.Errorf("qtype: invalid qtype: %v", v)
		}
	}
	return nil
}

func (r *itemMatcherQTypeRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemMatcherQTypeRule) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	question := dnsCtx.ReqMsg().Question[0]
	qType := question.Qtype
	for _, t := range r.qType {
		if t == qType {
			logger.DebugfContext(ctx, "qtype: match qtype: %s", dns.TypeToString[t])
			return true, nil
		}
	}
	logger.DebugfContext(ctx, "qtype: no match qtype: %s", dns.TypeToString[qType])
	return false, nil
}
