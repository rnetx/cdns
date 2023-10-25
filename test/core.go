package main

import (
	"context"
	"os"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"github.com/logrusorgru/aurora/v4"
)

var simpleCore *SimpleCore

func init() {
	ctx := context.Background()
	logger := log.NewSimpleLogger(os.Stdout, log.LevelDebug, false, false)
	simpleCore = NewSimpleCore(ctx, logger)
}

var _ adapter.Core = (*SimpleCore)(nil)

type SimpleCore struct {
	ctx               context.Context
	rootLogger        log.Logger
	coreLogger        log.Logger
	Listeners         []adapter.Listener
	ListenerMap       map[string]adapter.Listener
	Upstreams         []adapter.Upstream
	UpstreamMap       map[string]adapter.Upstream
	Workflows         []adapter.Workflow
	WorkflowMap       map[string]adapter.Workflow
	PluginMatchers    []adapter.PluginMatcher
	PluginMatcherMap  map[string]adapter.PluginMatcher
	PluginExecutors   []adapter.PluginExecutor
	PluginExecutorMap map[string]adapter.PluginExecutor
}

func NewSimpleCore(ctx context.Context, logger log.Logger) *SimpleCore {
	return &SimpleCore{
		ctx:               ctx,
		rootLogger:        logger,
		coreLogger:        log.NewTagLogger(logger, "core", aurora.RedFg),
		Listeners:         make([]adapter.Listener, 0),
		ListenerMap:       make(map[string]adapter.Listener),
		Upstreams:         make([]adapter.Upstream, 0),
		UpstreamMap:       make(map[string]adapter.Upstream),
		Workflows:         make([]adapter.Workflow, 0),
		WorkflowMap:       make(map[string]adapter.Workflow),
		PluginMatchers:    make([]adapter.PluginMatcher, 0),
		PluginMatcherMap:  make(map[string]adapter.PluginMatcher),
		PluginExecutors:   make([]adapter.PluginExecutor, 0),
		PluginExecutorMap: make(map[string]adapter.PluginExecutor),
	}
}

func (c *SimpleCore) Run() error {
	return nil
}

func (c *SimpleCore) Close() error {
	return nil
}

func (c *SimpleCore) GetListener(tag string) adapter.Listener {
	return c.ListenerMap[tag]
}

func (c *SimpleCore) GetListeners() []adapter.Listener {
	return c.Listeners
}

func (c *SimpleCore) GetUpstream(tag string) adapter.Upstream {
	return c.UpstreamMap[tag]
}

func (c *SimpleCore) GetUpstreams() []adapter.Upstream {
	return c.Upstreams
}

func (c *SimpleCore) GetWorkflow(tag string) adapter.Workflow {
	return c.WorkflowMap[tag]
}

func (c *SimpleCore) GetWorkflows() []adapter.Workflow {
	return c.Workflows
}

func (c *SimpleCore) GetPluginMatcher(tag string) adapter.PluginMatcher {
	return c.PluginMatcherMap[tag]
}

func (c *SimpleCore) GetPluginMatchers() []adapter.PluginMatcher {
	return c.PluginMatchers
}

func (c *SimpleCore) GetPluginExecutor(tag string) adapter.PluginExecutor {
	return c.PluginExecutorMap[tag]
}

func (c *SimpleCore) GetPluginExecutors() []adapter.PluginExecutor {
	return c.PluginExecutors
}

func (c *SimpleCore) AddListener(listener adapter.Listener) {
	c.Listeners = append(c.Listeners, listener)
	c.ListenerMap[listener.Tag()] = listener
}

func (c *SimpleCore) AddUpstream(upstream adapter.Upstream) {
	c.Upstreams = append(c.Upstreams, upstream)
	c.UpstreamMap[upstream.Tag()] = upstream
}

func (c *SimpleCore) AddWorkflow(workflow adapter.Workflow) {
	c.Workflows = append(c.Workflows, workflow)
	c.WorkflowMap[workflow.Tag()] = workflow
}

func (c *SimpleCore) AddPluginMatcher(pluginMatcher adapter.PluginMatcher) {
	c.PluginMatchers = append(c.PluginMatchers, pluginMatcher)
	c.PluginMatcherMap[pluginMatcher.Tag()] = pluginMatcher
}

func (c *SimpleCore) AddPluginExecutor(pluginExecutor adapter.PluginExecutor) {
	c.PluginExecutors = append(c.PluginExecutors, pluginExecutor)
	c.PluginExecutorMap[pluginExecutor.Tag()] = pluginExecutor
}

func (c *SimpleCore) RemoveListener(tag string) {
	listener := c.ListenerMap[tag]
	if listener != nil {
		delete(c.ListenerMap, tag)
		for i, l := range c.Listeners {
			if l == listener {
				c.Listeners = append(c.Listeners[:i], c.Listeners[i+1:]...)
				break
			}
		}
	}
}

func (c *SimpleCore) RemoveUpstream(tag string) {
	upstream := c.UpstreamMap[tag]
	if upstream != nil {
		delete(c.UpstreamMap, tag)
		for i, u := range c.Upstreams {
			if u == upstream {
				c.Upstreams = append(c.Upstreams[:i], c.Upstreams[i+1:]...)
				break
			}
		}
	}
}

func (c *SimpleCore) RemoveWorkflow(tag string) {
	workflow := c.WorkflowMap[tag]
	if workflow != nil {
		delete(c.WorkflowMap, tag)
		for i, w := range c.Workflows {
			if w == workflow {
				c.Workflows = append(c.Workflows[:i], c.Workflows[i+1:]...)
				break
			}
		}
	}
}

func (c *SimpleCore) RemovePluginMatcher(tag string) {
	pluginMatcher := c.PluginMatcherMap[tag]
	if pluginMatcher != nil {
		delete(c.PluginMatcherMap, tag)
		for i, pm := range c.PluginMatchers {
			if pm == pluginMatcher {
				c.PluginMatchers = append(c.PluginMatchers[:i], c.PluginMatchers[i+1:]...)
				break
			}
		}
	}
}

func (c *SimpleCore) RemovePluginExecutor(tag string) {
	pluginExecutor := c.PluginExecutorMap[tag]
	if pluginExecutor != nil {
		delete(c.PluginExecutorMap, tag)
		for i, pe := range c.PluginExecutors {
			if pe == pluginExecutor {
				c.PluginExecutors = append(c.PluginExecutors[:i], c.PluginExecutors[i+1:]...)
				break
			}
		}
	}
}

func (c *SimpleCore) Context() context.Context {
	return c.ctx
}

func (c *SimpleCore) RootLogger() log.Logger {
	return c.rootLogger
}
