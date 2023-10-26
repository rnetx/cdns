package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

type itemMatcherRule interface {
	yaml.Unmarshaler
	check(ctx context.Context, core adapter.Core) error
	match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error)
}

type RuleItemMatch struct {
	rule   itemMatcherRule
	invert bool
}

type RuleItemMatchOptions struct {
	Listener   yaml.Node `yaml:"listener,omitempty"`
	ClientIP   yaml.Node `yaml:"client-ip,omitempty"`
	QType      yaml.Node `yaml:"qtype,omitempty"`
	QName      yaml.Node `yaml:"qname,omitempty"`
	HasRespMsg yaml.Node `yaml:"has-resp-msg,omitempty"`
	RespIP     yaml.Node `yaml:"resp-ip,omitempty"`
	Mark       yaml.Node `yaml:"mark,omitempty"`
	Env        yaml.Node `yaml:"env,omitempty"`
	Metadata   yaml.Node `yaml:"metadata,omitempty"`
	Plugin     yaml.Node `yaml:"plugin,omitempty"`
	MatchOr    yaml.Node `yaml:"match-or,omitempty"`
	MatchAnd   yaml.Node `yaml:"match-and,omitempty"`
	//
	Invert bool `yaml:"invert,omitempty"`
}

func (r *RuleItemMatch) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var o RuleItemMatchOptions
	err := unmarshal(&o)
	if err != nil {
		return err
	}
	var item itemMatcherRule
	switch {
	case !o.Listener.IsZero():
		item = &itemMatcherListenerRule{}
		err = o.Listener.Decode(item)
	case !o.ClientIP.IsZero():
		item = &itemMatcherClientIPRule{}
		err = o.ClientIP.Decode(item)
	case !o.QType.IsZero():
		item = &itemMatcherQTypeRule{}
		err = o.QType.Decode(item)
	case !o.QName.IsZero():
		item = &itemMatcherQNameRule{}
		err = o.QName.Decode(item)
	case !o.HasRespMsg.IsZero():
		item = &itemMatcherHasRespMsgRule{}
		err = o.HasRespMsg.Decode(item)
	case !o.RespIP.IsZero():
		item = &itemMatcherRespIPRule{}
		err = o.RespIP.Decode(item)
	case !o.Mark.IsZero():
		item = &itemMatcherMarkRule{}
		err = o.Mark.Decode(item)
	case !o.Env.IsZero():
		item = &itemMatcherEnvRule{}
		err = o.Env.Decode(item)
	case !o.Metadata.IsZero():
		item = &itemMatcherMetadataRule{}
		err = o.Metadata.Decode(item)
	case !o.Plugin.IsZero():
		item = &itemMatcherPluginMatcherRule{}
		err = o.Plugin.Decode(item)
	case !o.MatchOr.IsZero():
		item = &itemMatcherMatchOrRule{}
		err = o.MatchOr.Decode(item)
	case !o.MatchAnd.IsZero():
		item = &itemMatcherMatchAndRule{}
		err = o.MatchAnd.Decode(item)
	default:
		return fmt.Errorf("match rule: unknown rule")
	}
	if err != nil {
		return err
	}
	r.rule = item
	r.invert = o.Invert
	return nil
}

func (r *RuleItemMatch) check(ctx context.Context, core adapter.Core) error {
	return r.rule.check(ctx, core)
}

func (r *RuleItemMatch) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	matched, err := r.rule.match(ctx, core, logger, dnsCtx)
	if err != nil {
		return matched, err
	}
	if r.invert {
		logger.DebugfContext(ctx, "invert match: %t => %t", matched, !matched)
		return !matched, nil
	}
	return matched, nil
}
