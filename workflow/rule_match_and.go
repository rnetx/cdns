package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

var _ Rule = (*RuleMatchAnd)(nil)

type RuleMatchAnd struct {
	MatchAnds []*RuleItemMatch
	ElseExecs []*RuleItemExec
	Execs     []*RuleItemExec
}

type RuleMatchAndOptions struct {
	MatchAnds []yaml.Node `yaml:"match-and,omitempty"`
	ElseExecs []yaml.Node `yaml:"else-exec,omitempty"`
	Execs     []yaml.Node `yaml:"exec,omitempty"`
}

func (r *RuleMatchAnd) UnmarshalYAML(unmarshal func(any) error) error {
	var o RuleMatchAndOptions
	err := unmarshal(&o)
	if err != nil {
		return err
	}
	if len(o.MatchAnds) == 0 {
		return fmt.Errorf("missing match-and")
	}
	if len(o.Execs) == 0 && len(o.ElseExecs) == 0 {
		return fmt.Errorf("missing exec or(and) else-exec")
	}
	matchAnds := make([]*RuleItemMatch, 0, len(o.MatchAnds))
	for i, node := range o.MatchAnds {
		if node.IsZero() {
			return fmt.Errorf("invalid match-and[%d]: empty", i)
		}
		var m RuleItemMatch
		err := node.Decode(&m)
		if err != nil {
			return fmt.Errorf("invalid match-and[%d]: %w", i, err)
		}
		matchAnds = append(matchAnds, &m)
	}
	r.MatchAnds = matchAnds
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

func (r *RuleMatchAnd) Check(ctx context.Context, core adapter.Core) error {
	var err error
	for i, m := range r.MatchAnds {
		err = m.check(ctx, core)
		if err != nil {
			return fmt.Errorf("match-and[%d]: %w", i, err)
		}
	}
	for i, e := range r.ElseExecs {
		err = e.check(ctx, core)
		if err != nil {
			return fmt.Errorf("else-exec[%d]: %w", i, err)
		}
	}
	for i, e := range r.Execs {
		err = e.check(ctx, core)
		if err != nil {
			return fmt.Errorf("exec[%d]: %w", i, err)
		}
	}
	return nil
}

func (r *RuleMatchAnd) Exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	match := true
	for i, m := range r.MatchAnds {
		logger.DebugfContext(ctx, "run match-and[%d]", i)
		matched, err := m.match(ctx, core, logger, dnsCtx)
		if err != nil {
			logger.ErrorfContext(ctx, "run match-and[%d]: run failed: %s", i, err)
			return adapter.ReturnModeUnknown, err
		}
		if !matched {
			logger.DebugfContext(ctx, "run match-and[%d]: no match", i)
			match = false
			break
		}
		logger.DebugfContext(ctx, "run match-and[%d]: matched, continue", i)
	}
	logger.DebugfContext(ctx, "run match-and finish")
	if match {
		if len(r.Execs) > 0 {
			for i, e := range r.Execs {
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
		if len(r.ElseExecs) > 0 {
			for i, e := range r.ElseExecs {
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
