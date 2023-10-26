package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

type itemExecutorRule interface {
	yaml.Unmarshaler
	check(ctx context.Context, core adapter.Core) error
	exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error)
}

type RuleItemExec struct {
	rule itemExecutorRule
}

type RuleItemExecOptions struct {
	Mark          yaml.Node `yaml:"mark,omitempty"`
	Metadata      yaml.Node `yaml:"metadata,omitempty"`
	Plugin        yaml.Node `yaml:"plugin,omitempty"`
	Upstream      yaml.Node `yaml:"upstream,omitempty"`
	JumpTo        yaml.Node `yaml:"jump-to,omitempty"`
	GoTo          yaml.Node `yaml:"go-to,omitempty"`
	WorkflowRules yaml.Node `yaml:"workflow-rules,omitempty"`
	Fallback      yaml.Node `yaml:"fallback,omitempty"`
	Parallel      yaml.Node `yaml:"parallel,omitempty"`
	SetTTL        yaml.Node `yaml:"set-ttl,omitempty"`
	Clean         yaml.Node `yaml:"clean,omitempty"`
	Return        yaml.Node `yaml:"return,omitempty"`
}

func (r *RuleItemExec) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var o RuleItemExecOptions
	err := unmarshal(&o)
	if err != nil {
		// String
		var s string
		err2 := unmarshal(&s)
		if err2 == nil {
			switch s {
			case "clean":
				r.rule = &itemExecutorCleanRule{
					clean: true,
				}
				return nil
			case "return":
				r.rule = &itemExecutorReturnRule{
					_return: "return",
				}
				return nil
			}
		}
		return err
	}
	var item itemExecutorRule
	switch {
	case !o.Mark.IsZero():
		item = &itemExecutorMarkRule{}
		err = o.Mark.Decode(item)
	case !o.Metadata.IsZero():
		item = &itemExecutorMetadataRule{}
		err = o.Metadata.Decode(item)
	case !o.Plugin.IsZero():
		item = &itemExecutorPluginExecutorRule{}
		err = o.Plugin.Decode(item)
	case !o.Upstream.IsZero():
		item = &itemExecutorUpstreamRule{}
		err = o.Upstream.Decode(item)
	case !o.JumpTo.IsZero():
		item = &itemExecutorJumpToRule{}
		err = o.JumpTo.Decode(item)
	case !o.GoTo.IsZero():
		item = &itemExecutorGoToRule{}
		err = o.GoTo.Decode(item)
	case !o.WorkflowRules.IsZero():
		item = &itemExecutorWorkflowRulesRule{}
		err = o.WorkflowRules.Decode(item)
	case !o.Fallback.IsZero():
		item = &itemExecutorFallbackRule{}
		err = o.Fallback.Decode(item)
	case !o.Parallel.IsZero():
		item = &itemExecutorParallelRule{}
		err = o.Parallel.Decode(item)
	case !o.SetTTL.IsZero():
		item = &itemExecutorSetTTLRule{}
		err = o.SetTTL.Decode(item)
	case !o.Clean.IsZero():
		item = &itemExecutorCleanRule{}
		err = o.Clean.Decode(item)
	case !o.Return.IsZero():
		item = &itemExecutorReturnRule{}
		err = o.Return.Decode(item)
	default:
		return fmt.Errorf("exec rule: unknown rule")
	}
	if err != nil {
		return err
	}
	r.rule = item
	return nil
}

func (r *RuleItemExec) check(ctx context.Context, core adapter.Core) error {
	return r.rule.check(ctx, core)
}

func (r *RuleItemExec) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	return r.rule.exec(ctx, core, logger, dnsCtx)
}
