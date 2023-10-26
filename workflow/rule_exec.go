package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

var _ Rule = (*RuleExec)(nil)

type RuleExec struct {
	Execs []*RuleItemExec
}

type RuleExecOptions struct {
	Execs []yaml.Node `yaml:"exec,omitempty"`
}

func (r *RuleExec) UnmarshalYAML(unmarshal func(any) error) error {
	var o RuleExecOptions
	err := unmarshal(&o)
	if err != nil {
		return err
	}
	if len(o.Execs) == 0 {
		return fmt.Errorf("missing exec")
	}
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
	return nil
}

func (r *RuleExec) Check(ctx context.Context, core adapter.Core) error {
	var err error
	for i, e := range r.Execs {
		err = e.check(ctx, core)
		if err != nil {
			return fmt.Errorf("exec[%d]: %w", i, err)
		}
	}
	return nil
}

func (r *RuleExec) Exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
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
}
