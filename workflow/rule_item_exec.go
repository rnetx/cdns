package workflow

import (
	"context"
	"fmt"
	"strings"

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
	setTTL        *uint32
	clean         bool
	_return       string
}

type _RuleItemExec struct {
	Mark          *uint64                     `yaml:"mark,omitempty"`
	Metadata      map[string]string           `yaml:"metadata,omitempty"`
	Plugin        yaml.Node                   `yaml:"plugin,omitempty"`
	Upstream      string                      `yaml:"upstream,omitempty"`
	JumpTo        utils.Listable[string]      `yaml:"jump-to,omitempty"`
	GoTo          string                      `yaml:"go-to,omitempty"`
	WorkflowRules utils.Listable[RuleOptions] `yaml:"workflow-rules,omitempty"`
	SetTTL        *uint32                     `yaml:"set-ttl,omitempty"`
	Clean         bool                        `yaml:"clean,omitempty"`
	Return        any                         `yaml:"return,omitempty"`
}

func (r *RuleItemExec) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var _r _RuleItemExec
	err := unmarshal(&_r)
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
	if _r.Mark != nil {
		r.mark = new(uint64)
		*r.mark = *_r.Mark
		tag++
	}
	if _r.Metadata != nil && len(_r.Metadata) > 0 {
		r.metadata = _r.Metadata
		tag++
	}
	if !_r.Plugin.IsZero() {
		r.plugin = &PluginExecutor{}
		err := _r.Plugin.Decode(&r.plugin)
		if err != nil {
			return fmt.Errorf("invalid plugin: %v", err)
		}
		tag++
	}
	if _r.Upstream != "" {
		r.upstreamTag = _r.Upstream
		tag++
	}
	if len(_r.JumpTo) > 0 {
		r.jumpToTag = _r.JumpTo
		tag++
	}
	if _r.GoTo != "" {
		r.goToTag = _r.GoTo
		tag++
	}
	if len(_r.WorkflowRules) > 0 {
		r.workflowRules = make([]Rule, 0, len(_r.WorkflowRules))
		for _, w := range _r.WorkflowRules {
			r.workflowRules = append(r.workflowRules, w.rule)
		}
		tag++
	}
	if _r.SetTTL != nil {
		r.setTTL = new(uint32)
		*r.setTTL = *_r.SetTTL
		tag++
	}
	if _r.Clean {
		r.clean = true
		tag++
	}
	if _r.Return != nil {
		switch rr := _r.Return.(type) {
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
