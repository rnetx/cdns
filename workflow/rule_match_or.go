package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

var _ Rule = (*RuleMatchOr)(nil)

type RuleMatchOr struct {
	MatchOrs  []*RuleItemMatch
	ElseExecs []*RuleItemExec
	Execs     []*RuleItemExec
}

type RuleMatchOrOptions struct {
	MatchOrs  []yaml.Node `yaml:"match-or,omitempty"`
	ElseExecs []yaml.Node `yaml:"else-exec,omitempty"`
	Execs     []yaml.Node `yaml:"exec,omitempty"`
}

func (r *RuleMatchOr) UnmarshalYAML(unmarshal func(any) error) error {
	var o RuleMatchOrOptions
	err := unmarshal(&o)
	if err != nil {
		return err
	}
	if len(o.MatchOrs) == 0 {
		return fmt.Errorf("missing match-or")
	}
	if len(o.Execs) == 0 && len(o.ElseExecs) == 0 {
		return fmt.Errorf("missing exec or(and) else-exec")
	}
	matchOrs := make([]*RuleItemMatch, 0, len(o.MatchOrs))
	for i, node := range o.MatchOrs {
		if node.IsZero() {
			return fmt.Errorf("invalid match-or[%d]: empty", i)
		}
		var m RuleItemMatch
		err := node.Decode(&m)
		if err != nil {
			return fmt.Errorf("invalid match-or[%d]: %w", i, err)
		}
		matchOrs = append(matchOrs, &m)
	}
	r.MatchOrs = matchOrs
	if len(o.ElseExecs) > 0 {
		elseExecs := make([]*RuleItemExec, 0, len(o.ElseExecs))
		for i, node := range o.ElseExecs {
			if node.IsZero() {
				return fmt.Errorf("invalid else-exec[%d]: empty", i)
			}
			var e RuleItemExec
			err := node.Decode(&e)
			if err != nil {
				return fmt.Errorf("invalid else-exec[%d]: %w", i, err)
			}
			elseExecs = append(elseExecs, &e)
		}
		r.ElseExecs = elseExecs
	}
	if len(o.Execs) > 0 {
		execs := make([]*RuleItemExec, 0, len(o.Execs))
		for i, node := range o.Execs {
			if node.IsZero() {
				return fmt.Errorf("invalid exec[%d]: empty", i)
			}
			var e RuleItemExec
			err := node.Decode(&e)
			if err != nil {
				return fmt.Errorf("invalid exec[%d]: %w", i, err)
			}
			execs = append(execs, &e)
		}
		r.Execs = execs
	}
	return nil
}

func (o *RuleMatchOr) Check(ctx context.Context, core adapter.Core) error {
	var err error
	for i, m := range o.MatchOrs {
		err = m.check(ctx, core)
		if err != nil {
			return fmt.Errorf("match-or[%d]: %w", i, err)
		}
	}
	for i, e := range o.ElseExecs {
		err = e.check(ctx, core)
		if err != nil {
			return fmt.Errorf("else-exec[%d]: %w", i, err)
		}
	}
	for i, e := range o.Execs {
		err = e.check(ctx, core)
		if err != nil {
			return fmt.Errorf("exec[%d]: %w", i, err)
		}
	}
	return nil
}

func (o *RuleMatchOr) Exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	match := false
	for i, m := range o.MatchOrs {
		logger.DebugfContext(ctx, "run match-or[%d]", i)
		matched, err := m.match(ctx, core, logger, dnsCtx)
		if err != nil {
			logger.ErrorfContext(ctx, "run match-or[%d]: run failed: %s", i, err)
			return adapter.ReturnModeUnknown, err
		}
		if matched {
			logger.DebugfContext(ctx, "run match-or[%d]: matched", i)
			match = true
			break
		}
		logger.DebugfContext(ctx, "run match-or[%d]: no match, continue", i)
	}
	logger.DebugfContext(ctx, "run match-or finish")
	if match {
		if len(o.Execs) > 0 {
			for i, e := range o.Execs {
				logger.DebugfContext(ctx, "run exec[%d]", i)
				returnMode, err := e.exec(ctx, core, logger, dnsCtx)
				if err != nil {
					logger.ErrorfContext(ctx, "run exec[%d]: run failed: %s", i, err)
					return adapter.ReturnModeUnknown, err
				}
				if returnMode != adapter.ReturnModeContinue {
					logger.DebugfContext(ctx, "run exec[%d]: %s", i, returnMode.String())
					return returnMode, nil
				}
			}
			logger.DebugfContext(ctx, "run exec finish")
			return adapter.ReturnModeContinue, nil
		} else {
			returnMode := adapter.ReturnModeContinue
			logger.DebugfContext(ctx, "no exec, %s", returnMode.String())
			return returnMode, nil
		}
	} else {
		if len(o.ElseExecs) > 0 {
			for i, e := range o.ElseExecs {
				logger.DebugfContext(ctx, "run else-exec[%d]", i)
				returnMode, err := e.exec(ctx, core, logger, dnsCtx)
				if err != nil {
					logger.ErrorfContext(ctx, "run else-exec[%d]: run failed: %s", i, err)
					return adapter.ReturnModeUnknown, err
				}
				if returnMode != adapter.ReturnModeContinue {
					logger.DebugfContext(ctx, "run else-exec[%d]: %s", i, returnMode.String())
					return returnMode, nil
				}
			}
			logger.DebugfContext(ctx, "run else-exec finish")
			return adapter.ReturnModeContinue, nil
		} else {
			returnMode := adapter.ReturnModeContinue
			logger.DebugfContext(ctx, "no else-exec, %s", returnMode.String())
			return returnMode, nil
		}
	}
}
