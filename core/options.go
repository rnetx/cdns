package core

import (
	"github.com/rnetx/cdns/api"
	"github.com/rnetx/cdns/listener"
	"github.com/rnetx/cdns/ntp"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/upstream"
	"github.com/rnetx/cdns/workflow"
)

type Options struct {
	Log             LogOptions                     `yaml:"log,omitempty"`
	API             *api.Options                   `yaml:"api,omitempty"`
	Upstreams       []upstream.Options             `yaml:"upstreams,omitempty"`
	Workflows       []workflow.WorkflowOptions     `yaml:"workflows,omitempty"`
	Listeners       []listener.Options             `yaml:"listeners,omitempty"`
	PluginMatchers  []plugin.PluginMatcherOptions  `yaml:"plugin-matchers,omitempty"`
	PluginExecutors []plugin.PluginExecutorOptions `yaml:"plugin-executors,omitempty"`
	NTP             *ntp.NTPOptions                `yaml:"ntp,omitempty"`
}

type LogOptions struct {
	Disabled         bool   `yaml:"disabled,omitempty"`
	Level            string `yaml:"level,omitempty"`
	Output           string `yaml:"output,omitempty"`
	DisableTimestamp bool   `yaml:"disable-timestamp,omitempty"`
	DisableColor     bool   `yaml:"disable-color,omitempty"`
}
