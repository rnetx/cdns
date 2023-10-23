package plugin

import (
	"context"
	"fmt"
	"sync"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
)

type PluginExecutorOptions struct {
	Tag  string `yaml:"tag"`
	Type string `yaml:"type"`
	Args any    `yaml:"args,omitempty"`
}

var executorMap sync.Map

type PluginExecutorFactory func(ctx context.Context, core adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginExecutor, error)

func RegisterPluginExecutor(_type string, factory PluginExecutorFactory) {
	executorMap.Store(_type, factory)
}

func NewPluginExecutor(ctx context.Context, core adapter.Core, logger log.Logger, tag string, _type string, args any) (adapter.PluginExecutor, error) {
	v, ok := executorMap.Load(_type)
	if !ok {
		return nil, fmt.Errorf("unknown plugin executor type: %s", _type)
	}
	f := v.(PluginExecutorFactory)
	return f(ctx, core, logger, tag, args)
}

func PluginExecutorTypes() []string {
	var types []string
	executorMap.Range(func(key any, value any) bool {
		types = append(types, key.(string))
		return true
	})
	return types
}
