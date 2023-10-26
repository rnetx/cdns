package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"gopkg.in/yaml.v3"
)

var _ itemExecutorRule = (*itemExecutorWorkflowRulesRule)(nil)

type itemExecutorWorkflowRulesRule struct {
	rules []Rule
}

func (r *itemExecutorWorkflowRulesRule) UnmarshalYAML(value *yaml.Node) error {
	var w utils.Listable[RuleOptions]
	err := value.Decode(&w)
	if err != nil {
		return fmt.Errorf("workflow-rules: %w", err)
	}
	if len(w) == 0 {
		return fmt.Errorf("workflow-rules: missing workflow-rules")
	}
	r.rules = make([]Rule, 0, len(w))
	for _, o := range w {
		r.rules = append(r.rules, o.rule)
	}
	return nil
}

func (r *itemExecutorWorkflowRulesRule) check(ctx context.Context, core adapter.Core) error {
	for i, w := range r.rules {
		err := w.Check(ctx, core)
		if err != nil {
			return fmt.Errorf("workflow-rules: workflow-rule[%d] check failed: %v", i, err)
		}
	}
	return nil
}

func (r *itemExecutorWorkflowRulesRule) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	for i, w := range r.rules {
		logger.DebugfContext(ctx, "workflow-rules: workflow-rule[%d] exec", i)
		returnMode, err := w.Exec(ctx, core, logger, dnsCtx)
		if err != nil {
			logger.ErrorfContext(ctx, "workflow-rules: workflow-rule[%d] exec failed: %v", i, err)
			return adapter.ReturnModeUnknown, err
		}
		switch returnMode {
		case adapter.ReturnModeReturnAll:
			logger.DebugfContext(ctx, "workflow-rules: workflow-rule[%d]: %s", i, returnMode.String())
			return returnMode, nil
		case adapter.ReturnModeReturnOnce:
			logger.DebugfContext(ctx, "workflow-rules: workflow-rule[%d]: %s", i, returnMode.String())
			return adapter.ReturnModeContinue, nil
		}
	}
	return adapter.ReturnModeContinue, nil
}
