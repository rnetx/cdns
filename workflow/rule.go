package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
)

type Rule interface {
	Check(ctx context.Context, core adapter.Core) error
	Exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error)
}

type RuleOptions struct {
	rule Rule
}

type _RuleOptions struct {
	MatchOr  any `yaml:"match-or,omitempty"`
	MatchAnd any `yaml:"match-and,omitempty"`
	ElseExec any `yaml:"else-exec,omitempty"`
	Exec     any `yaml:"exec,omitempty"`
}

func (r *RuleOptions) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var o _RuleOptions
	err := unmarshal(&o)
	if err != nil {
		return err
	}
	var (
		isMatchOr  = o.MatchOr != nil
		isMatchAnd = o.MatchAnd != nil
		isExec     = o.Exec != nil
	)
	switch {
	case isMatchOr && isExec:
		r.rule = &RuleMatchOr{}
	case isMatchAnd && isExec:
		r.rule = &RuleMatchAnd{}
	case isExec:
		r.rule = &RuleExec{}
	default:
		return fmt.Errorf("invalid workflow rule")
	}
	return unmarshal(r.rule)
}
