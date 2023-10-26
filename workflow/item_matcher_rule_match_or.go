package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"gopkg.in/yaml.v3"
)

var _ itemMatcherRule = (*itemMatcherMatchOrRule)(nil)

type itemMatcherMatchOrRule struct {
	matchOr []RuleItemMatch
}

func (r *itemMatcherMatchOrRule) UnmarshalYAML(value *yaml.Node) error {
	var m utils.Listable[RuleItemMatch]
	err := value.Decode(&m)
	if err != nil {
		return fmt.Errorf("match-or: %w", err)
	}
	if len(m) == 0 {
		return fmt.Errorf("match-or: missing match rule")
	}
	r.matchOr = m
	return nil
}

func (r *itemMatcherMatchOrRule) check(ctx context.Context, core adapter.Core) error {
	for i, m := range r.matchOr {
		err := m.check(ctx, core)
		if err != nil {
			return fmt.Errorf("match-or: match-or[%d] check failed: %v", i, err)
		}
	}
	return nil
}

func (r *itemMatcherMatchOrRule) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	match := false
	for i, m := range r.matchOr {
		logger.DebugfContext(ctx, "match-or: match match-or[%d]", i)
		matched, err := m.match(ctx, core, logger, dnsCtx)
		if err != nil {
			logger.DebugfContext(ctx, "match-or: match match-or[%d] failed: %v", i, err)
			return false, err
		}
		if matched {
			logger.DebugfContext(ctx, "match-or: match match-or[%d] => true", i)
			match = true
			break
		}
		logger.DebugfContext(ctx, "match-or: match match-or[%d] => false, continue", i)
	}
	if match {
		logger.DebugfContext(ctx, "match-or: match match-or: true")
		return true, nil
	} else {
		logger.DebugfContext(ctx, "match-or: no match match-or")
		return false, nil
	}
}
