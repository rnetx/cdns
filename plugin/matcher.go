package plugin

import (
	"context"
	"fmt"
	"sync"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
)

type PluginMatcherOptions struct {
	Tag  string `yaml:"tag"`
	Type string `yaml:"type"`
	Args any    `yaml:"args,omitempty"`
}

var matcherMap sync.Map

type PluginMatcherFactory func(ctx context.Context, core adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginMatcher, error)

func RegisterPluginMatcher(_type string, factory PluginMatcherFactory) {
	matcherMap.Store(_type, factory)
}

func NewPluginMatcher(ctx context.Context, core adapter.Core, logger log.Logger, tag string, _type string, args any) (adapter.PluginMatcher, error) {
	v, ok := matcherMap.Load(_type)
	if !ok {
		return nil, fmt.Errorf("unknown plugin matcher type: %s", _type)
	}
	f := v.(PluginMatcherFactory)
	return f(ctx, core, logger, tag, args)
}

func PluginMatcherTypes() []string {
	var types []string
	matcherMap.Range(func(key any, value any) bool {
		types = append(types, key.(string))
		return true
	})
	return types
}
