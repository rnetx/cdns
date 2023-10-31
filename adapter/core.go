package adapter

import "time"

type Core interface {
	Closer
	Run() error
	GetListener(tag string) Listener
	GetListeners() []Listener
	GetUpstream(tag string) Upstream
	GetUpstreams() []Upstream
	GetWorkflow(tag string) Workflow
	GetWorkflows() []Workflow
	GetPluginMatcher(tag string) PluginMatcher
	GetPluginMatchers() []PluginMatcher
	GetPluginExecutor(tag string) PluginExecutor
	GetPluginExecutors() []PluginExecutor
	GetTimeFunc() func() time.Time
}
