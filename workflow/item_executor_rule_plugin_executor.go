package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

var _ itemExecutorRule = (*itemExecutorPluginExecutorRule)(nil)

type itemExecutorPluginExecutorRule struct {
	tag      string
	args     any
	argsID   uint16
	executor adapter.PluginExecutor
}

type itemExecutorPluginExecutorRuleOptions struct {
	Tag  string `yaml:"tag,omitempty"`
	Args any    `yaml:"args,omitempty"`
}

func (r *itemExecutorPluginExecutorRule) UnmarshalYAML(value *yaml.Node) error {
	var o itemExecutorPluginExecutorRuleOptions
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

func (r *itemExecutorPluginExecutorRule) check(ctx context.Context, core adapter.Core) error {
	p := core.GetPluginExecutor(r.tag)
	if p == nil {
		return fmt.Errorf("plugin: plugin executor [%s] not found", r.tag)
	}
	id, err := p.LoadRunningArgs(ctx, r.args)
	if err != nil {
		return fmt.Errorf("plugin: plugin executor [%s] load running args failed: %v", r.tag, err)
	}
	r.argsID = id
	r.executor = p
	r.tag = ""   // clean
	r.args = nil // clean
	return nil
}

func (r *itemExecutorPluginExecutorRule) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	returnMode, err := r.executor.Exec(ctx, dnsCtx, r.argsID)
	if err != nil {
		logger.DebugfContext(ctx, "plugin: plugin executor [%s] exec failed: %v", r.executor.Tag(), err)
		return adapter.ReturnModeUnknown, err
	}
	logger.DebugfContext(ctx, "plugin: plugin executor [%s]: %s", r.executor.Tag(), returnMode.String())
	return returnMode, nil
}
