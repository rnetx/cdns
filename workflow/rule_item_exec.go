package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
)

type RuleItemExec struct {
	mark          *uint64
	metadata      map[string]string
	plugin        *PluginExecutor
	upstreamTag   string
	upstream      adapter.Upstream
	jumpToTag     []string
	jumpTo        []adapter.Workflow
	goToTag       string
	goTo          adapter.Workflow
	workflowRules []Rule
	fallback      *Fallback
	parallel      *Parallel
	setTTL        *uint32
	clean         bool
	_return       string
}

type RuleItemExecOptions struct {
	Mark          *uint64                     `yaml:"mark,omitempty"`
	Metadata      map[string]string           `yaml:"metadata,omitempty"`
	Plugin        yaml.Node                   `yaml:"plugin,omitempty"`
	Upstream      string                      `yaml:"upstream,omitempty"`
	JumpTo        utils.Listable[string]      `yaml:"jump-to,omitempty"`
	GoTo          string                      `yaml:"go-to,omitempty"`
	WorkflowRules utils.Listable[RuleOptions] `yaml:"workflow-rules,omitempty"`
	Fallback      *FallbackOptions            `yaml:"fallback,omitempty"`
	Parallel      *ParallelOptions            `yaml:"parallel,omitempty"`
	SetTTL        *uint32                     `yaml:"set-ttl,omitempty"`
	Clean         bool                        `yaml:"clean,omitempty"`
	Return        any                         `yaml:"return,omitempty"`
}

func (r *RuleItemExec) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var o RuleItemExecOptions
	err := unmarshal(&o)
	if err != nil {
		// String
		var s string
		err2 := unmarshal(&s)
		if err2 == nil {
			switch s {
			case "clean":
				r.clean = true
				return nil
			case "return":
				r._return = "all"
				return nil
			}
		}
		return err
	}
	tag := 0
	if o.Mark != nil {
		r.mark = new(uint64)
		*r.mark = *o.Mark
		tag++
	}
	if o.Metadata != nil && len(o.Metadata) > 0 {
		r.metadata = o.Metadata
		tag++
	}
	if !o.Plugin.IsZero() {
		r.plugin = &PluginExecutor{}
		err := o.Plugin.Decode(&r.plugin)
		if err != nil {
			return fmt.Errorf("invalid plugin: %v", err)
		}
		tag++
	}
	if o.Upstream != "" {
		r.upstreamTag = o.Upstream
		tag++
	}
	if len(o.JumpTo) > 0 {
		r.jumpToTag = o.JumpTo
		tag++
	}
	if o.GoTo != "" {
		r.goToTag = o.GoTo
		tag++
	}
	if len(o.WorkflowRules) > 0 {
		r.workflowRules = make([]Rule, 0, len(o.WorkflowRules))
		for _, w := range o.WorkflowRules {
			r.workflowRules = append(r.workflowRules, w.rule)
		}
		tag++
	}
	if o.Fallback != nil {
		fallback := &Fallback{}
		var mainTag int
		if o.Fallback.Main != nil && len(o.Fallback.Main) > 0 {
			fallback.main = make([]Rule, 0, len(o.Fallback.Main))
			for _, w := range o.Fallback.Main {
				fallback.main = append(fallback.main, w.rule)
			}
			mainTag++
		}
		if o.Fallback.MainWorkflow != "" {
			fallback.mainWorkflowTag = o.Fallback.MainWorkflow
			mainTag++
		}
		if mainTag != 1 {
			return fmt.Errorf("main and main-workflow must be set one")
		}
		var fallbackTag int
		if o.Fallback.Fallback != nil && len(o.Fallback.Fallback) > 0 {
			fallback.fallback = make([]Rule, 0, len(o.Fallback.Fallback))
			for _, w := range o.Fallback.Fallback {
				fallback.fallback = append(fallback.fallback, w.rule)
			}
			fallbackTag++
		}
		if o.Fallback.FallbackWorkflow != "" {
			fallback.fallbackWorkflowTag = o.Fallback.FallbackWorkflow
			fallbackTag++
		}
		if fallbackTag != 1 {
			return fmt.Errorf("fallback and fallback-workflow must be set one")
		}
		fallback.alwaysStandby = o.Fallback.AlwaysStandby
		if o.Fallback.WaitTime > 0 {
			fallback.waitTime = time.Duration(o.Fallback.WaitTime)
			if fallback.alwaysStandby {
				return fmt.Errorf("always-standby and wait-time must be set one")
			}
		}
		r.fallback = fallback
		tag++
	}
	if o.Parallel != nil && len(o.Parallel.Workflows) > 0 {
		r.parallel = &Parallel{
			workflowTags: o.Parallel.Workflows,
		}
		tag++
	}
	if o.SetTTL != nil {
		r.setTTL = new(uint32)
		*r.setTTL = *o.SetTTL
		tag++
	}
	if o.Clean {
		r.clean = true
		tag++
	}
	if o.Return != nil {
		switch rr := o.Return.(type) {
		case string:
			rr = strings.ToLower(rr)
			switch rr {
			case "all":
				r._return = "all"
			case "once":
				r._return = "once"
			case "success":
				r._return = "success" // all
			case "failure", "fail":
				r._return = "failure" // all
			case "nxdomain":
				r._return = "nxdomain" // all
			case "refused":
				r._return = "refused" // all
			default:
				return fmt.Errorf("invalid return")
			}
		default:
			return fmt.Errorf("invalid return")
		}
		tag++
	}
	if tag != 1 {
		return fmt.Errorf("invalid exec rule")
	}
	return nil
}

func (r *RuleItemExec) check(ctx context.Context, core adapter.Core) error {
	if r.plugin != nil {
		p := core.GetPluginExecutor(r.plugin.tag)
		if p == nil {
			return fmt.Errorf("plugin executor [%s] not found", r.plugin.tag)
		}
		id, err := p.LoadRunningArgs(ctx, r.plugin.args)
		if err != nil {
			return fmt.Errorf("plugin executor [%s] load running args failed: %v", r.plugin.tag, err)
		}
		r.plugin.argsID = id
		r.plugin.executor = p
		r.plugin.tag = ""   // clean
		r.plugin.args = nil // clean
	}
	if r.upstreamTag != "" {
		u := core.GetUpstream(r.upstreamTag)
		if u == nil {
			return fmt.Errorf("upstream [%s] not found", r.upstreamTag)
		}
		r.upstream = u
		r.upstreamTag = "" // clean
	}
	if len(r.jumpToTag) > 0 {
		r.jumpTo = make([]adapter.Workflow, 0, len(r.jumpToTag))
		for _, tag := range r.jumpToTag {
			w := core.GetWorkflow(tag)
			if w == nil {
				return fmt.Errorf("workflow [%s] not found", tag)
			}
			r.jumpTo = append(r.jumpTo, w)
		}
		r.jumpToTag = nil // clean
	}
	if r.goToTag != "" {
		w := core.GetWorkflow(r.goToTag)
		if w == nil {
			return fmt.Errorf("workflow [%s] not found", r.goToTag)
		}
		r.goTo = w
		r.goToTag = "" // clean
	}
	if len(r.workflowRules) > 0 {
		for i, w := range r.workflowRules {
			err := w.Check(ctx, core)
			if err != nil {
				return fmt.Errorf("workflow-rule [%d] check failed: %v", i, err)
			}
		}
	}
	if r.fallback != nil {
		if r.fallback.mainWorkflowTag != "" {
			w := core.GetWorkflow(r.fallback.mainWorkflowTag)
			if w == nil {
				return fmt.Errorf("fallback: main workflow [%s] not found", r.fallback.mainWorkflowTag)
			}
			r.fallback.mainWorkflow = w
			r.fallback.mainWorkflowTag = "" // clean
		}
		if r.fallback.fallbackWorkflowTag != "" {
			w := core.GetWorkflow(r.fallback.fallbackWorkflowTag)
			if w == nil {
				return fmt.Errorf("fallback: fallback workflow [%s] not found", r.fallback.fallbackWorkflowTag)
			}
			r.fallback.fallbackWorkflow = w
			r.fallback.fallbackWorkflowTag = "" // clean
		}
	}
	if r.parallel != nil {
		r.parallel.workflows = make([]adapter.Workflow, 0, len(r.parallel.workflowTags))
		for _, tag := range r.parallel.workflowTags {
			w := core.GetWorkflow(tag)
			if w == nil {
				return fmt.Errorf("parallel: workflow [%s] not found", tag)
			}
			r.parallel.workflows = append(r.parallel.workflows, w)
		}
		r.parallel.workflowTags = nil // clean
	}
	return nil
}

func (r *RuleItemExec) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	if r.mark != nil {
		mark := dnsCtx.Mark()
		dnsCtx.SetMark(*r.mark)
		logger.DebugfContext(ctx, "set mark: %d => %d", mark, *r.mark)
		return adapter.ReturnModeContinue, nil
	}
	if r.metadata != nil && len(r.metadata) > 0 {
		metadata := dnsCtx.Metadata()
		for k, v := range r.metadata {
			if v == "" {
				logger.DebugfContext(ctx, "delete metadata: %s", k)
				delete(metadata, k)
			} else {
				vv, ok := metadata[k]
				if ok {
					logger.DebugfContext(ctx, "set metadata: key: %s, value: %s => %s", k, vv, v)
				} else {
					logger.DebugfContext(ctx, "set metadata: key: %s, value: %s", k, v)
				}
				metadata[k] = v
			}
		}
		return adapter.ReturnModeContinue, nil
	}
	if r.plugin != nil {
		returnMode, err := r.plugin.executor.Exec(ctx, dnsCtx, r.plugin.argsID)
		if err != nil {
			logger.DebugfContext(ctx, "plugin executor [%s] exec failed: %v", r.plugin.executor.Tag(), err)
			return adapter.ReturnModeUnknown, err
		}
		logger.DebugfContext(ctx, "plugin executor [%s]: %s", r.plugin.executor.Tag(), returnMode.String())
		return returnMode, nil
	}
	if r.upstream != nil {
		reqMsg := dnsCtx.ReqMsg()
		if reqMsg == nil {
			logger.DebugfContext(ctx, "upstream: request message is nil")
			return adapter.ReturnModeContinue, nil
		}
		exchangeHooks := dnsCtx.ExchangeHooks()
		defer dnsCtx.FlushExchangeHooks()
		for i, exchangeHook := range exchangeHooks {
			logger.DebugfContext(ctx, "exchange hook [%d]: before hook run", i)
			returnMode, err := exchangeHook.BeforeExchange(ctx, dnsCtx, reqMsg)
			if err != nil {
				logger.DebugfContext(ctx, "exchange hook [%d]: before hook run failed: %v", i, err)
				return adapter.ReturnModeUnknown, err
			}
			if returnMode != adapter.ReturnModeContinue {
				logger.DebugfContext(ctx, "exchange hook [%d]: before hook run: %s", i, returnMode.String())
				return returnMode, nil
			}
		}
		respMsg, err := r.upstream.Exchange(ctx, reqMsg)
		if err != nil {
			logger.DebugfContext(ctx, "upstream [%s] exchange failed: %v", r.upstream.Tag(), err)
			return adapter.ReturnModeUnknown, err
		}
		dnsCtx.SetRespMsg(respMsg)
		dnsCtx.SetRespUpstreamTag(r.upstream.Tag())
		extraExchanges := dnsCtx.ExtraExchanges()
		if len(extraExchanges) > 0 {
			ch := utils.NewSafeChan[exchangeResult](len(extraExchanges))
			defer ch.Close()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			for i, extraExchange := range extraExchanges {
				go func(ch *utils.SafeChan[exchangeResult], index int, reqMsg *dns.Msg) {
					respMsg, err := r.upstream.Exchange(ctx, reqMsg)
					if err != nil {
						select {
						case ch.SendChan() <- exchangeResult{
							index: index,
							req:   reqMsg,
							resp:  nil,
							err:   err,
						}:
						case <-ctx.Done():
						}
					} else {
						select {
						case ch.SendChan() <- exchangeResult{
							index: index,
							req:   reqMsg,
							resp:  respMsg,
						}:
						case <-ctx.Done():
						}
					}
				}(ch.Clone(), i, extraExchange.Req)
			}
			for i := 0; i < len(extraExchanges); i++ {
				select {
				case res := <-ch.ReceiveChan():
					if res.err == nil {
						extraExchanges[res.index].Resp = res.resp
					}
				case <-ctx.Done():
				}
			}
			dnsCtx.SetExtraExchanges(extraExchanges)
		}
		for i, exchangeHook := range exchangeHooks {
			logger.DebugfContext(ctx, "exchange hook [%d]: after hook run", i)
			returnMode, err := exchangeHook.AfterExchange(ctx, dnsCtx, reqMsg, respMsg)
			if err != nil {
				logger.DebugfContext(ctx, "exchange hook [%d]: after hook run failed: %v", i, err)
				return adapter.ReturnModeUnknown, err
			}
			if returnMode != adapter.ReturnModeContinue {
				logger.DebugfContext(ctx, "exchange hook [%d]: after hook run: %s", i, returnMode.String())
				return returnMode, nil
			}
		}
		return adapter.ReturnModeContinue, nil
	}
	if len(r.jumpTo) > 0 {
		for _, w := range r.jumpTo {
			logger.DebugfContext(ctx, "jump to workflow [%s]", w.Tag())
			returnMode, err := w.Exec(ctx, dnsCtx)
			if err != nil {
				logger.DebugfContext(ctx, "workflow [%s] exec failed: %v", w.Tag(), err)
				return adapter.ReturnModeUnknown, err
			}
			logger.DebugfContext(ctx, "workflow [%s]: %s", w.Tag(), returnMode.String())
			if returnMode != adapter.ReturnModeContinue {
				return returnMode, nil
			}
		}
		return adapter.ReturnModeContinue, nil
	}
	if r.goTo != nil {
		logger.DebugfContext(ctx, "go to workflow [%s]", r.goTo.Tag())
		return r.goTo.Exec(ctx, dnsCtx)
	}
	if len(r.workflowRules) > 0 {
		for i, w := range r.workflowRules {
			logger.DebugfContext(ctx, "workflow-rule [%d] exec", i)
			returnMode, err := w.Exec(ctx, core, logger, dnsCtx)
			if err != nil {
				logger.ErrorfContext(ctx, "workflow-rule [%d] exec failed: %v", i, err)
				return adapter.ReturnModeUnknown, err
			}
			switch returnMode {
			case adapter.ReturnModeReturnAll:
				logger.DebugfContext(ctx, "workflow-rule [%d]: %s", i, returnMode.String())
				return returnMode, nil
			case adapter.ReturnModeReturnOnce:
				logger.DebugfContext(ctx, "workflow-rule [%d]: %s", i, returnMode.String())
				return adapter.ReturnModeContinue, nil
			}
		}
		return adapter.ReturnModeContinue, nil
	}
	if r.fallback != nil {
		ch := utils.NewSafeChan[fallbackResult](1)
		defer ch.Close()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		var (
			waitFallbackCtx  context.Context
			waitFallbackFunc context.CancelFunc
		)
		if !r.fallback.alwaysStandby && r.fallback.waitTime > 0 {
			waitFallbackCtx, waitFallbackFunc = context.WithCancel(ctx)
			defer waitFallbackFunc()
		}
		mainDNSCtx := dnsCtx.Clone()
		mainDNSCtx.SetID(mainDNSCtx.ID() + 1)
		mainDNSCtx.FlushColor()
		if r.fallback.mainWorkflow != nil {
			go func(
				ctx context.Context,
				ch *utils.SafeChan[fallbackResult],
				dnsCtx *adapter.DNSContext,
				logger log.Logger,
				w adapter.Workflow,
			) {
				defer ch.Close()
				logger.DebugfContext(ctx, "fallback: main workflow [%s] exec, id: %d", w.Tag(), dnsCtx.ID())
				returnMode, err := w.Exec(adapter.SaveLogContext(ctx, dnsCtx), dnsCtx)
				if err != nil {
					logger.DebugfContext(ctx, "fallback: main workflow [%s] exec failed: %v", w.Tag(), err)
				} else {
					logger.DebugfContext(ctx, "fallback: main workflow [%s] exec success", w.Tag())
					select {
					case ch.SendChan() <- fallbackResult{
						dnsCtx:     dnsCtx,
						returnMode: returnMode,
					}:
					default:
					}
				}
			}(
				ctx,
				ch.Clone(),
				mainDNSCtx,
				logger,
				r.fallback.mainWorkflow,
			)
		} else {
			go func(
				ctx context.Context,
				ch *utils.SafeChan[fallbackResult],
				dnsCtx *adapter.DNSContext,
				core adapter.Core,
				logger log.Logger,
				rules []Rule,
			) {
				defer ch.Close()
				logger.DebugfContext(ctx, "fallback: main rules exec, id: %d", dnsCtx.ID())
				var (
					returnMode adapter.ReturnMode
					err        error
				)
				rCtx := adapter.SaveLogContext(ctx, dnsCtx)
				for i, r := range rules {
					logger.DebugfContext(ctx, "fallback: main rule[%d] exec", i)
					returnMode, err = r.Exec(rCtx, core, logger, dnsCtx)
					if err != nil {
						logger.DebugfContext(ctx, "fallback: main rule[%d] exec failed: %v", i, err)
						return
					}
					if returnMode != adapter.ReturnModeContinue {
						logger.DebugfContext(ctx, "fallback: main rule[%d] exec success: %s", i, returnMode.String())
						break
					}
				}
				logger.DebugfContext(ctx, "fallback: main rules exec success: %s", returnMode.String())
				select {
				case ch.SendChan() <- fallbackResult{
					dnsCtx:     dnsCtx,
					returnMode: returnMode,
				}:
				default:
				}
			}(
				ctx,
				ch.Clone(),
				mainDNSCtx,
				core,
				logger,
				r.fallback.main,
			)
		}
		fallbackDNSCtx := dnsCtx.Clone()
		fallbackDNSCtx.SetID(fallbackDNSCtx.ID() + 2)
		fallbackDNSCtx.FlushColor()
		if r.fallback.fallbackWorkflow != nil {
			go func(
				ctx context.Context,
				ch *utils.SafeChan[fallbackResult],
				dnsCtx *adapter.DNSContext,
				logger log.Logger,
				w adapter.Workflow,
				alwaysStandby bool,
				waitTime time.Duration,
				waitFallbackCtx context.Context,
			) {
				defer ch.Close()
				logger.DebugfContext(ctx, "fallback: fallback workflow [%s] exec, id: %d", w.Tag(), dnsCtx.ID())
				if !alwaysStandby && waitTime > 0 {
					logger.DebugfContext(ctx, "fallback: fallback workflow [%s] waiting...", w.Tag())
					select {
					case <-waitFallbackCtx.Done():
						select {
						case <-ctx.Done():
							return
						default:
						}
					case <-ctx.Done():
						return
					}
				}
				returnMode, err := w.Exec(adapter.SaveLogContext(ctx, dnsCtx), dnsCtx)
				if err != nil {
					logger.DebugfContext(ctx, "fallback: fallback workflow [%s] exec failed: %v", w.Tag(), err)
				} else {
					logger.DebugfContext(ctx, "fallback: fallback workflow [%s] exec success", w.Tag())
					select {
					case ch.SendChan() <- fallbackResult{
						dnsCtx:     dnsCtx,
						returnMode: returnMode,
						isFallback: true,
					}:
					default:
					}
				}
			}(
				ctx,
				ch.Clone(),
				fallbackDNSCtx,
				logger,
				r.fallback.fallbackWorkflow,
				r.fallback.alwaysStandby,
				r.fallback.waitTime,
				waitFallbackCtx,
			)
		} else {
			go func(
				ctx context.Context,
				ch *utils.SafeChan[fallbackResult],
				dnsCtx *adapter.DNSContext,
				core adapter.Core,
				logger log.Logger,
				rules []Rule,
				alwaysStandby bool,
				waitTime time.Duration,
				waitFallbackCtx context.Context,
			) {
				defer ch.Close()
				logger.DebugfContext(ctx, "fallback: fallback rules exec, id: %d", dnsCtx.ID())
				if !alwaysStandby && waitTime > 0 {
					logger.DebugContext(ctx, "fallback: fallback rules waiting...")
					select {
					case <-waitFallbackCtx.Done():
						select {
						case <-ctx.Done():
							return
						default:
						}
					case <-ctx.Done():
						return
					}
				}
				var (
					returnMode adapter.ReturnMode
					err        error
				)
				rCtx := adapter.SaveLogContext(ctx, dnsCtx)
				for i, r := range rules {
					logger.DebugfContext(ctx, "fallback: fallback rule[%d] exec", i)
					returnMode, err = r.Exec(rCtx, core, logger, dnsCtx)
					if err != nil {
						logger.DebugfContext(ctx, "fallback: fallback rule[%d] exec failed: %v", i, err)
						return
					}
					if returnMode != adapter.ReturnModeContinue {
						logger.DebugfContext(ctx, "fallback: fallback rule[%d] exec success: %s", returnMode.String())
						break
					}
				}
				logger.DebugfContext(ctx, "fallback: fallback rules exec success: %s", returnMode.String())
				select {
				case ch.SendChan() <- fallbackResult{
					dnsCtx:     dnsCtx,
					returnMode: returnMode,
					isFallback: true,
				}:
				default:
				}
			}(
				ctx,
				ch.Clone(),
				fallbackDNSCtx,
				core,
				logger,
				r.fallback.fallback,
				r.fallback.alwaysStandby,
				r.fallback.waitTime,
				waitFallbackCtx,
			)
		}
		select {
		case result := <-ch.ReceiveChan():
			if !result.isFallback {
				logger.DebugfContext(ctx, "fallback: main exec success: %s", result.returnMode.String())
			} else {
				logger.DebugfContext(ctx, "fallback: fallback exec success: %s", result.returnMode.String())
			}
			oldID := dnsCtx.ID()
			*dnsCtx = *result.dnsCtx
			dnsCtx.SetID(oldID)
			dnsCtx.FlushColor()
			return result.returnMode, nil
		case <-ctx.Done():
			logger.DebugfContext(ctx, "fallback: timeout")
			return adapter.ReturnModeUnknown, ctx.Err()
		}
	}
	if r.parallel != nil {
		ch := utils.NewSafeChan[parallelResult](1)
		defer ch.Close()
		taskGroup := utils.NewTaskGroupWithContext(ctx)
		defer taskGroup.Close()
		for i, w := range r.parallel.workflows {
			iDNSCtx := dnsCtx.Clone()
			iDNSCtx.SetID(iDNSCtx.ID() + uint32(i) + 1)
			iDNSCtx.FlushColor()
			logger.DebugfContext(ctx, "parallel: workflow [%s] exec, id: %d", w.Tag(), iDNSCtx.ID())
			go func(
				ctx context.Context,
				task *utils.Task,
				dnsCtx *adapter.DNSContext,
				ch *utils.SafeChan[parallelResult],
				w adapter.Workflow,
			) {
				defer task.Finish()
				defer ch.Close()
				returnMode, err := w.Exec(adapter.SaveLogContext(ctx, dnsCtx), dnsCtx)
				if err == nil {
					select {
					case ch.SendChan() <- parallelResult{
						w:          w,
						dnsCtx:     dnsCtx,
						returnMode: returnMode,
					}:
					default:
					}
				}
			}(
				ctx,
				taskGroup.AddTask(),
				iDNSCtx,
				ch.Clone(),
				w,
			)
		}
		select {
		case result := <-ch.ReceiveChan():
			logger.DebugfContext(ctx, "parallel: workflow [%s] exec success: %s", result.w.Tag(), result.returnMode.String())
			oldID := dnsCtx.ID()
			*dnsCtx = *result.dnsCtx
			dnsCtx.SetID(oldID)
			dnsCtx.FlushColor()
			return result.returnMode, nil
		case <-ctx.Done():
			logger.ErrorContext(ctx, "parallel: timeout")
			return adapter.ReturnModeUnknown, ctx.Err()
		case <-taskGroup.Wait():
			err := fmt.Errorf("parallel: all workflow exec failed")
			logger.ErrorContext(ctx, err)
			return adapter.ReturnModeUnknown, err
		}
	}
	if r.setTTL != nil {
		respMsg := dnsCtx.RespMsg()
		if respMsg == nil {
			logger.DebugfContext(ctx, "set ttl: response message is nil")
			return adapter.ReturnModeContinue, nil
		}
		for i := range respMsg.Answer {
			respMsg.Answer[i].Header().Ttl = *r.setTTL
		}
		logger.DebugfContext(ctx, "set ttl: %d", *r.setTTL)
		return adapter.ReturnModeContinue, nil
	}
	if r.clean {
		dnsCtx.SetRespMsg(nil)
		logger.DebugContext(ctx, "clean response message")
		return adapter.ReturnModeContinue, nil
	}
	if r._return != "" {
		var rcode int
		switch r._return {
		case "all":
			logger.DebugContext(ctx, "return all")
			return adapter.ReturnModeReturnAll, nil
		case "once":
			logger.DebugContext(ctx, "return once")
			return adapter.ReturnModeReturnOnce, nil
		case "success":
			logger.DebugContext(ctx, "return success")
			rcode = dns.RcodeSuccess
		case "failure":
			logger.DebugContext(ctx, "return failure")
			rcode = dns.RcodeServerFailure
		case "nxdomain":
			logger.DebugContext(ctx, "return nxdomain")
			rcode = dns.RcodeNameError
		case "refused":
			logger.DebugContext(ctx, "return refused")
			rcode = dns.RcodeRefused
		}
		var name string
		question := dnsCtx.ReqMsg().Question
		if len(question) > 0 {
			name = question[0].Name
		}
		newRespMsg := &dns.Msg{}
		newRespMsg.SetRcode(dnsCtx.ReqMsg(), rcode)
		newRespMsg.Ns = []dns.RR{utils.FakeSOA(name)}
		return adapter.ReturnModeReturnAll, nil
	}
	panic("unreachable")
}

type exchangeResult struct {
	index int
	req   *dns.Msg
	resp  *dns.Msg
	err   error
}

type fallbackResult struct {
	isFallback bool
	dnsCtx     *adapter.DNSContext
	returnMode adapter.ReturnMode
}

type Fallback struct {
	main                []Rule
	fallback            []Rule
	mainWorkflowTag     string
	mainWorkflow        adapter.Workflow
	fallbackWorkflowTag string
	fallbackWorkflow    adapter.Workflow
	alwaysStandby       bool
	waitTime            time.Duration
}

type FallbackOptions struct {
	Main             utils.Listable[RuleOptions] `yaml:"main,omitempty"`
	Fallback         utils.Listable[RuleOptions] `yaml:"fallback,omitempty"`
	MainWorkflow     string                      `yaml:"main-workflow,omitempty"`
	FallbackWorkflow string                      `yaml:"fallback-workflow,omitempty"`
	AlwaysStandby    bool                        `yaml:"always-standby,omitempty"`
	WaitTime         utils.Duration              `yaml:"wait-time,omitempty"`
}

type parallelResult struct {
	w          adapter.Workflow
	dnsCtx     *adapter.DNSContext
	returnMode adapter.ReturnMode
}

type Parallel struct {
	workflowTags []string
	workflows    []adapter.Workflow
}

type ParallelOptions struct {
	Workflows utils.Listable[string] `yaml:"workflows,omitempty"`
}
