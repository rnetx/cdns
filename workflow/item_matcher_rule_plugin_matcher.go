package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

var _ itemMatcherRule = (*itemMatcherPluginMatcherRule)(nil)

type itemMatcherPluginMatcherRule struct {
	tag     string
	args    any
	argsID  uint16
	matcher adapter.PluginMatcher
}

type itemMatcherPluginMatcherRuleOptions struct {
	Tag  string `yaml:"tag,omitempty"`
	Args any    `yaml:"args,omitempty"`
}

func (r *itemMatcherPluginMatcherRule) UnmarshalYAML(value *yaml.Node) error {
	var o itemMatcherPluginMatcherRuleOptions
	err := value.Decode(&o)
	if err != nil {
		return fmt.Errorf("plugin: %w", err)
	}
	if o.Tag == "" {
		return fmt.Errorf("plugin: missing tag")
	}
	r.tag = o.Tag
	r.args = o.Args
	return nil
}

func (r *itemMatcherPluginMatcherRule) check(ctx context.Context, core adapter.Core) error {
	p := core.GetPluginMatcher(r.tag)
	if p == nil {
		return fmt.Errorf("plugin: plugin matcher [%s] not found", r.tag)
	}
	id, err := p.LoadRunningArgs(ctx, r.args)
	if err != nil {
		return fmt.Errorf("plugin: plugin matcher [%s] load running args failed: %v", r.tag, err)
	}
	r.argsID = id
	r.matcher = p
	r.tag = ""   // clean
	r.args = nil // clean
	return nil
}

func (r *itemMatcherPluginMatcherRule) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	matched, err := r.matcher.Match(ctx, dnsCtx, r.argsID)
	if err != nil {
		logger.DebugfContext(ctx, "plugin: plugin matcher [%s] match failed: %v", r.matcher.Tag(), err)
		return false, err
	}
	logger.DebugfContext(ctx, "plugin: plugin matcher [%s] match: %t", r.matcher.Tag(), matched)
	return matched, nil
}
