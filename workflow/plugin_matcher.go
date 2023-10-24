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

type _PluginMatcher struct {
	Tag  string `yaml:"tag,omitempty"`
	Args any    `yaml:"args,omitempty"`
}

func (p *PluginMatcher) UnmarshalYAML(unmarshal func(any) error) error {
	var _p _PluginMatcher
	err := unmarshal(&_p)
	if err != nil {
		return err
	}
	if _p.Tag == "" {
		return fmt.Errorf("missing tag")
	}
	p.tag = _p.Tag
	p.args = _p.Args
	return nil
}
