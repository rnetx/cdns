package workflow

import (
	"fmt"

	"github.com/rnetx/cdns/adapter"
)

type PluginMatcher struct {
	tag     string
	args    any
	argsID  uint16
	matcher adapter.PluginMatcher
}

type PluginMatcherOptions struct {
	Tag  string `yaml:"tag,omitempty"`
	Args any    `yaml:"args,omitempty"`
}

func (p *PluginMatcher) UnmarshalYAML(unmarshal func(any) error) error {
	var o PluginMatcherOptions
	err := unmarshal(&o)
	if err != nil {
		return err
	}
	if o.Tag == "" {
		return fmt.Errorf("missing tag")
	}
	p.tag = o.Tag
	p.args = o.Args
	return nil
}
