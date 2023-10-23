package workflow

import (
	"fmt"

	"github.com/rnetx/cdns/adapter"
)

type PluginExecutor struct {
	tag      string
	args     any
	argsID   uint64
	executor adapter.PluginExecutor
}

type _PluginExecutor struct {
	Tag  string `yaml:"tag,omitempty"`
	Args any    `yaml:"args,omitempty"`
}

func (p *PluginExecutor) UnmarshalYAML(unmarshal func(any) error) error {
	var _p _PluginExecutor
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
