package core

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/logrusorgru/aurora/v4"
	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/listener"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/plugin/executor"
	"github.com/rnetx/cdns/plugin/matcher"
	"github.com/rnetx/cdns/upstream"
	"github.com/rnetx/cdns/utils"
	"github.com/rnetx/cdns/workflow"
)

func init() {
	matcher.Do()
	executor.Do()
}

var _ adapter.Core = (*Core)(nil)

type Core struct {
	ctx         context.Context
	rootLogger  log.Logger
	coreLogger  log.Logger
	closeOutput io.Closer
	//
	listeners         []adapter.Listener
	listenerMap       map[string]adapter.Listener
	upstreams         []adapter.Upstream
	upstreamMap       map[string]adapter.Upstream
	workflows         []adapter.Workflow
	workflowMap       map[string]adapter.Workflow
	pluginMatchers    []adapter.PluginMatcher
	pluginMatcherMap  map[string]adapter.PluginMatcher
	pluginExecutors   []adapter.PluginExecutor
	pluginExecutorMap map[string]adapter.PluginExecutor
}

func NewCore(ctx context.Context, options Options) (adapter.Core, log.Logger, error) {
	level := log.LevelInfo
	switch options.Log.Level {
	case "debug", "Debug":
		level = log.LevelDebug
	case "info", "Info", "":
		level = log.LevelInfo
	case "warn", "Warn", "warning", "Warning":
		level = log.LevelWarn
	case "error", "Error":
		level = log.LevelError
	case "fatal", "Fatal":
		level = log.LevelFatal
	default:
		return nil, nil, fmt.Errorf("invalid log level: %s", options.Log.Level)
	}
	var logOutput io.Writer
	switch options.Log.Output {
	case "stdout", "Stdout", "":
		logOutput = os.Stdout
	case "stderr", "Stderr":
		logOutput = os.Stderr
	default:
		options.Log.DisableColor = true
		f, err := os.OpenFile(options.Log.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file failed: %s", err)
		}
		logOutput = f
	}
	rootLogger := log.NewSimpleLogger(logOutput, level, options.Log.DisableTimestamp, options.Log.DisableColor)
	c := &Core{
		ctx:        ctx,
		rootLogger: rootLogger,
		coreLogger: log.NewTagLogger(rootLogger, "core", aurora.RedFg),
	}
	if closer, isCloser := logOutput.(io.Closer); isCloser {
		c.closeOutput = closer
	}
	if len(options.Upstreams) == 0 {
		return nil, nil, fmt.Errorf("missing upstreams")
	}
	c.upstreams = make([]adapter.Upstream, 0, len(options.Upstreams))
	c.upstreamMap = make(map[string]adapter.Upstream, len(options.Upstreams))
	for i, upstreamOptions := range options.Upstreams {
		tag := upstreamOptions.Tag
		if tag == "" {
			return nil, nil, fmt.Errorf("create upstream[%d] failed: missing upstream tag", i)
		}
		_, ok := c.upstreamMap[tag]
		if ok {
			return nil, nil, fmt.Errorf("create upstream[%d] failed: duplicate upstream tag: %s", i, tag)
		}
		upstreamLogger := log.NewTagLogger(c.rootLogger, fmt.Sprintf("upstream/%s", tag), aurora.GreenFg)
		u, err := upstream.NewUpstream(c.ctx, c, upstreamLogger, tag, upstreamOptions)
		if err != nil {
			return nil, nil, fmt.Errorf("create upstream[%d] failed: %s", i, err)
		}
		c.upstreams = append(c.upstreams, u)
		c.upstreamMap[tag] = u
	}
	var err error
	c.upstreams, err = sortUpstream(c.upstreams)
	if err != nil {
		return nil, nil, fmt.Errorf("sort upstreams failed: %s", err)
	}
	if len(options.Workflows) == 0 {
		return nil, nil, fmt.Errorf("missing workflows")
	}
	c.workflows = make([]adapter.Workflow, 0, len(options.Workflows))
	c.workflowMap = make(map[string]adapter.Workflow, len(options.Workflows))
	for i, workflowOptions := range options.Workflows {
		tag := workflowOptions.Tag
		if tag == "" {
			return nil, nil, fmt.Errorf("create workflow[%d] failed: missing workflow tag", i)
		}
		_, ok := c.workflowMap[tag]
		if ok {
			return nil, nil, fmt.Errorf("create workflow[%d] failed: duplicate workflow tag: %s", i, tag)
		}
		workflowLogger := log.NewTagLogger(c.rootLogger, fmt.Sprintf("workflow/%s", tag), aurora.CyanFg)
		w, err := workflow.NewWorkflow(c.ctx, c, workflowLogger, tag, workflowOptions)
		if err != nil {
			return nil, nil, fmt.Errorf("create workflow[%d] failed: %s", i, err)
		}
		c.workflows = append(c.workflows, w)
		c.workflowMap[tag] = w
	}
	if len(options.Listeners) == 0 {
		return nil, nil, fmt.Errorf("missing listeners")
	}
	c.listeners = make([]adapter.Listener, 0, len(options.Listeners))
	c.listenerMap = make(map[string]adapter.Listener, len(options.Listeners))
	for i, listenerOptions := range options.Listeners {
		tag := listenerOptions.Tag
		if tag == "" {
			return nil, nil, fmt.Errorf("create listener[%d] failed: missing listener tag", i)
		}
		_, ok := c.listenerMap[tag]
		if ok {
			return nil, nil, fmt.Errorf("create listener[%d] failed: duplicate listener tag: %s", i, tag)
		}
		listenerLogger := log.NewTagLogger(c.rootLogger, fmt.Sprintf("listener/%s", tag), aurora.YellowFg)
		l, err := listener.NewListener(c.ctx, c, listenerLogger, tag, listenerOptions)
		if err != nil {
			return nil, nil, fmt.Errorf("create listener[%d] failed: %s", i, err)
		}
		c.listeners = append(c.listeners, l)
		c.listenerMap[tag] = l
	}
	if len(options.PluginMatchers) > 0 {
		c.pluginMatchers = make([]adapter.PluginMatcher, 0, len(options.PluginMatchers))
		c.pluginMatcherMap = make(map[string]adapter.PluginMatcher, len(options.PluginMatchers))
		for i, pluginMatcherOptions := range options.PluginMatchers {
			tag := pluginMatcherOptions.Tag
			if tag == "" {
				return nil, nil, fmt.Errorf("create plugin matcher[%d] failed: missing plugin matcher tag", i)
			}
			_, ok := c.pluginMatcherMap[tag]
			if ok {
				return nil, nil, fmt.Errorf("create plugin matcher[%d] failed: duplicate plugin matcher tag: %s", i, tag)
			}
			pluginMatcherLogger := log.NewTagLogger(c.rootLogger, fmt.Sprintf("plugin-matcher/%s", tag), aurora.MagentaFg)
			pm, err := plugin.NewPluginMatcher(c.ctx, c, pluginMatcherLogger, tag, pluginMatcherOptions.Type, pluginMatcherOptions.Args)
			if err != nil {
				return nil, nil, fmt.Errorf("create plugin matcher[%d] failed: %s", i, err)
			}
			c.pluginMatchers = append(c.pluginMatchers, pm)
			c.pluginMatcherMap[tag] = pm
		}
	}
	if len(options.PluginExecutors) > 0 {
		c.pluginExecutors = make([]adapter.PluginExecutor, 0, len(options.PluginExecutors))
		c.pluginExecutorMap = make(map[string]adapter.PluginExecutor, len(options.PluginExecutors))
		for i, pluginExecutorOptions := range options.PluginExecutors {
			tag := pluginExecutorOptions.Tag
			if tag == "" {
				return nil, nil, fmt.Errorf("create plugin executor[%d] failed: missing plugin executor tag", i)
			}
			_, ok := c.pluginExecutorMap[tag]
			if ok {
				return nil, nil, fmt.Errorf("create plugin executor[%d] failed: duplicate plugin executor tag: %s", i, tag)
			}
			pluginExecutorLogger := log.NewTagLogger(c.rootLogger, fmt.Sprintf("plugin-executor/%s", tag), aurora.BlueFg)
			pe, err := plugin.NewPluginExecutor(c.ctx, c, pluginExecutorLogger, tag, pluginExecutorOptions.Type, pluginExecutorOptions.Args)
			if err != nil {
				return nil, nil, fmt.Errorf("create plugin executor[%d] failed: %s", i, err)
			}
			c.pluginExecutors = append(c.pluginExecutors, pe)
			c.pluginExecutorMap[tag] = pe
		}
	}
	return c, c.coreLogger, nil
}

func (c *Core) Close() error {
	if c.closeOutput != nil {
		return c.closeOutput.Close()
	}
	return nil
}

func (c *Core) Run() error {
	c.coreLogger.Info("core is starting...")
	defer c.coreLogger.Info("core is stopped")
	t := time.Now()
	var err error
	// Upstreams
	upstreamStack := utils.NewStack[adapter.Upstream](len(c.upstreams))
	defer func() {
		var err error
		for upstreamStack.Len() > 0 {
			u := upstreamStack.Pop()
			closer, isCloser := u.(adapter.Closer)
			if isCloser {
				err = closer.Close()
				if err != nil {
					c.coreLogger.Errorf("close upstream[%s] failed: %s", u.Tag(), err)
				} else {
					c.coreLogger.Infof("close upstream[%s] success", u.Tag())
				}
			}
		}
	}()
	for _, u := range c.upstreams {
		starter, isStarter := u.(adapter.Starter)
		if isStarter {
			err = starter.Start()
			if err != nil {
				err = fmt.Errorf("start upstream[%s] failed: %s", u.Tag(), err)
				c.rootLogger.Fatal(err)
				return err
			}
		}
		upstreamStack.Push(u)
	}
	if len(c.pluginMatchers) > 0 {
		pluginMatcherStack := utils.NewStack[adapter.PluginMatcher](len(c.pluginMatchers))
		defer func() {
			var err error
			for pluginMatcherStack.Len() > 0 {
				pm := pluginMatcherStack.Pop()
				closer, isCloser := pm.(adapter.Closer)
				if isCloser {
					err = closer.Close()
					if err != nil {
						c.coreLogger.Errorf("close plugin matcher[%s] failed: %s", pm.Tag(), err)
					} else {
						c.coreLogger.Infof("close plugin matcher[%s] success", pm.Tag())
					}
				}
			}
		}()
		for _, pm := range c.pluginMatchers {
			starter, isStarter := pm.(adapter.Starter)
			if isStarter {
				err = starter.Start()
				if err != nil {
					err = fmt.Errorf("start plugin matcher[%s] failed: %s", pm.Tag(), err)
					c.rootLogger.Fatal(err)
					return err
				}
			}
			pluginMatcherStack.Push(pm)
		}
	}
	if len(c.pluginExecutors) > 0 {
		pluginExecutorStack := utils.NewStack[adapter.PluginExecutor](len(c.pluginExecutors))
		defer func() {
			var err error
			for pluginExecutorStack.Len() > 0 {
				pe := pluginExecutorStack.Pop()
				closer, isCloser := pe.(adapter.Closer)
				if isCloser {
					err = closer.Close()
					if err != nil {
						c.coreLogger.Errorf("close plugin executor[%s] failed: %s", pe.Tag(), err)
					} else {
						c.coreLogger.Infof("close plugin executor[%s] success", pe.Tag())
					}
				}
			}
		}()
		for _, pe := range c.pluginExecutors {
			starter, isStarter := pe.(adapter.Starter)
			if isStarter {
				err = starter.Start()
				if err != nil {
					err = fmt.Errorf("start plugin executor[%s] failed: %s", pe.Tag(), err)
					c.rootLogger.Fatal(err)
					return err
				}
			}
			pluginExecutorStack.Push(pe)
		}
	}
	for _, w := range c.workflows {
		err = w.Check()
		if err != nil {
			err = fmt.Errorf("check workflow[%s] failed: %s", w.Tag(), err)
			c.rootLogger.Fatal(err)
			return err
		}
	}
	listenerStack := utils.NewStack[adapter.Listener](len(c.listeners))
	defer func() {
		var err error
		for listenerStack.Len() > 0 {
			l := listenerStack.Pop()
			closer, isCloser := l.(adapter.Closer)
			if isCloser {
				err = closer.Close()
				if err != nil {
					c.coreLogger.Errorf("close listener[%s] failed: %s", l.Tag(), err)
				} else {
					c.coreLogger.Infof("close listener[%s] success", l.Tag())
				}
			}
		}
	}()
	for _, l := range c.listeners {
		starter, isStarter := l.(adapter.Starter)
		if isStarter {
			err = starter.Start()
			if err != nil {
				err = fmt.Errorf("start listener[%s] failed: %s", l.Tag(), err)
				c.rootLogger.Fatal(err)
				return err
			}
		}
		listenerStack.Push(l)
	}
	duration := time.Since(t)
	c.coreLogger.Infof("core is started, cost: %dms", duration.Milliseconds())
	<-c.ctx.Done()
	c.coreLogger.Info("core is stopping...")
	return nil
}

func (c *Core) GetListener(tag string) adapter.Listener {
	return c.listenerMap[tag]
}

func (c *Core) GetListeners() []adapter.Listener {
	return c.listeners
}

func (c *Core) GetUpstream(tag string) adapter.Upstream {
	return c.upstreamMap[tag]
}

func (c *Core) GetUpstreams() []adapter.Upstream {
	return c.upstreams
}

func (c *Core) GetWorkflow(tag string) adapter.Workflow {
	return c.workflowMap[tag]
}

func (c *Core) GetWorkflows() []adapter.Workflow {
	return c.workflows
}

func (c *Core) GetPluginMatcher(tag string) adapter.PluginMatcher {
	return c.pluginMatcherMap[tag]
}

func (c *Core) GetPluginMatchers() []adapter.PluginMatcher {
	return c.pluginMatchers
}

func (c *Core) GetPluginExecutor(tag string) adapter.PluginExecutor {
	return c.pluginExecutorMap[tag]
}

func (c *Core) GetPluginExecutors() []adapter.PluginExecutor {
	return c.pluginExecutors
}
