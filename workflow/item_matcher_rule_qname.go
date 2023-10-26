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

var _ itemMatcherRule = (*itemMatcherQNameRule)(nil)

type itemMatcherQNameRule struct {
	qName []string
}

func (r *itemMatcherQNameRule) UnmarshalYAML(value *yaml.Node) error {
	var q utils.Listable[string]
	err := value.Decode(&q)
	if err != nil {
		return fmt.Errorf("qname: %w", err)
	}
	if len(q) == 0 {
		return fmt.Errorf("qname: missing qname")
	}
	r.qName = make([]string, 0, len(q))
	for _, v := range q {
		r.qName = append(r.qName, dns.Fqdn(v))
	}
	return nil
}

func (r *itemMatcherQNameRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemMatcherQNameRule) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	question := dnsCtx.ReqMsg().Question
	if len(question) == 0 {
		logger.DebugfContext(ctx, "qname: no match qname: no request question found")
		return false, nil
	}
	qName := question[0].Name
	for _, n := range r.qName {
		if n == qName {
			logger.DebugfContext(ctx, "qname: match qname: %s", qName)
			return true, nil
		}
	}
	logger.DebugfContext(ctx, "qname: no match qname: %s", qName)
	return false, nil
}
