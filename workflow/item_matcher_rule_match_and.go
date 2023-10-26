package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"gopkg.in/yaml.v3"
)

var _ itemMatcherRule = (*itemMatcherMatchAndRule)(nil)

type itemMatcherMatchAndRule struct {
	matchAnd []RuleItemMatch
}

func (r *itemMatcherMatchAndRule) UnmarshalYAML(value *yaml.Node) error {
	var m utils.Listable[RuleItemMatch]
	err := value.Decode(&m)
	if err != nil {
		return fmt.Errorf("match-and: %w", err)
	}
	if len(m) == 0 {
		return fmt.Errorf("match-and: missing match rule")
	}
	r.matchAnd = m
	return nil
}

func (r *itemMatcherMatchAndRule) check(ctx context.Context, core adapter.Core) error {
	for i, m := range r.matchAnd {
		err := m.check(ctx, core)
		if err != nil {
			return fmt.Errorf("match-and: match-and[%d] check failed: %v", i, err)
		}
	}
	return nil
}

func (r *itemMatcherMatchAndRule) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	match := true
	for i, m := range r.matchAnd {
		logger.DebugfContext(ctx, "match-and: match match-and[%d]", i)
		matched, err := m.match(ctx, core, logger, dnsCtx)
		if err != nil {
			logger.DebugfContext(ctx, "match-and: match match-and[%d] failed: %v", i, err)
			return false, err
		}
		if !matched {
			logger.DebugfContext(ctx, "match-and: match match-and[%d] => false", i)
			match = false
			break
		}
		logger.DebugfContext(ctx, "match-and: match match-and[%d] => true, continue", i)
	}
	if match {
		logger.DebugfContext(ctx, "match-and: match match-and: true")
		return true, nil
	} else {
		logger.DebugfContext(ctx, "match-and: no match match-and")
		return false, nil
	}
}
