package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"
)

type WorkflowOptions struct {
	Tag   string                      `yaml:"tag"`
	Rules utils.Listable[RuleOptions] `yaml:"rules"`
}

type Workflow struct {
	ctx    context.Context
	tag    string
	core   adapter.Core
	logger log.Logger

	rules []Rule
}

func NewWorkflow(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options WorkflowOptions) (adapter.Workflow, error) {
	w := &Workflow{
		ctx:    ctx,
		tag:    tag,
		core:   core,
		logger: logger,
	}
	if len(options.Rules) == 0 {
		return nil, fmt.Errorf("missing rules")
	}
	w.rules = make([]Rule, 0, len(options.Rules))
	for _, o := range options.Rules {
		w.rules = append(w.rules, o.rule)
	}
	return w, nil
}

func (w *Workflow) Tag() string {
	return w.tag
}

func (w *Workflow) Check() error {
	var err error
	for i, rule := range w.rules {
		err = rule.Check(w.ctx, w.core)
		if err != nil {
			return fmt.Errorf("rule [%d] check failed: %v", i, err)
		}
	}
	return nil
}

func (w *Workflow) Exec(ctx context.Context, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	for i, rule := range w.rules {
		w.logger.DebugfContext(ctx, "rule[%d] exec", i)
		returnMode, err := rule.Exec(ctx, w.core, w.logger, dnsCtx)
		if err != nil {
			w.logger.ErrorfContext(ctx, "rule[%d] exec failed: %v", i, err)
			return adapter.ReturnModeUnknown, err
		}
		switch returnMode {
		case adapter.ReturnModeReturnAll:
			w.logger.DebugfContext(ctx, "rule[%d]: %s", i, returnMode.String())
			return returnMode, nil
		case adapter.ReturnModeReturnOnce:
			w.logger.DebugfContext(ctx, "rule[%d]: %s", i, returnMode.String())
			return adapter.ReturnModeContinue, nil
		}
	}
	return adapter.ReturnModeContinue, nil
}
