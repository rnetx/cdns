package workflow

import (
	"fmt"

	"github.com/rnetx/cdns/adapter"
)

type PluginExecutor struct {
	tag      string
	args     any
	argsID   uint16
	executor adapter.PluginExecutor
}

type PluginExecutorOptions struct {
	Tag  string `yaml:"tag,omitempty"`
	Args any    `yaml:"args,omitempty"`
}

func (p *PluginExecutor) UnmarshalYAML(unmarshal func(any) error) error {
	var o PluginExecutorOptions
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
