package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"gopkg.in/yaml.v3"
)

var _ itemExecutorRule = (*itemExecutorFallbackRule)(nil)

type itemExecutorFallbackRule struct {
	main                []Rule
	fallback            []Rule
	mainWorkflowTag     string
	mainWorkflow        adapter.Workflow
	fallbackWorkflowTag string
	fallbackWorkflow    adapter.Workflow
	alwaysStandby       bool
	waitTime            time.Duration
}

type itemExecutorFallbackRuleOptions struct {
	Main             utils.Listable[RuleOptions] `yaml:"main,omitempty"`
	Fallback         utils.Listable[RuleOptions] `yaml:"fallback,omitempty"`
	MainWorkflow     string                      `yaml:"main-workflow,omitempty"`
	FallbackWorkflow string                      `yaml:"fallback-workflow,omitempty"`
	AlwaysStandby    bool                        `yaml:"always-standby,omitempty"`
	WaitTime         utils.Duration              `yaml:"wait-time,omitempty"`
}

func (r *itemExecutorFallbackRule) UnmarshalYAML(value *yaml.Node) error {
	var o itemExecutorFallbackRuleOptions
	err := value.Decode(&o)
	if err != nil {
		return fmt.Errorf("fallback: %w", err)
	}
	var mainTag int
	if o.Main != nil && len(o.Main) > 0 {
		r.main = make([]Rule, 0, len(o.Main))
		for _, w := range o.Main {
			r.main = append(r.main, w.rule)
		}
		mainTag++
	}
	if o.MainWorkflow != "" {
		r.mainWorkflowTag = o.MainWorkflow
		mainTag++
	}
	if mainTag != 1 {
		return fmt.Errorf("fallback: main and main-workflow must be set one")
	}
	var fallbackTag int
	if o.Fallback != nil && len(o.Fallback) > 0 {
		r.fallback = make([]Rule, 0, len(o.Fallback))
		for _, w := range o.Fallback {
			r.fallback = append(r.fallback, w.rule)
		}
		fallbackTag++
	}
	if o.FallbackWorkflow != "" {
		r.fallbackWorkflowTag = o.FallbackWorkflow
		fallbackTag++
	}
	if fallbackTag != 1 {
		return fmt.Errorf("fallback: fallback and fallback-workflow must be set one")
	}
	r.alwaysStandby = o.AlwaysStandby
	if o.WaitTime > 0 {
		r.waitTime = time.Duration(o.WaitTime)
		if r.alwaysStandby {
			return fmt.Errorf("fallback: always-standby and wait-time must be set one")
		}
	}
	return nil
}

func (r *itemExecutorFallbackRule) check(ctx context.Context, core adapter.Core) error {
	if r.mainWorkflowTag != "" {
		w := core.GetWorkflow(r.mainWorkflowTag)
		if w == nil {
			return fmt.Errorf("fallback: main workflow [%s] not found", r.mainWorkflowTag)
		}
		r.mainWorkflow = w
		r.mainWorkflowTag = "" // clean
	}
	if r.fallbackWorkflowTag != "" {
		w := core.GetWorkflow(r.fallbackWorkflowTag)
		if w == nil {
			return fmt.Errorf("fallback: fallback workflow [%s] not found", r.fallbackWorkflowTag)
		}
		r.fallbackWorkflow = w
		r.fallbackWorkflowTag = "" // clean
	}
	return nil
}

func (r *itemExecutorFallbackRule) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	ch := utils.NewSafeChan[fallbackResult](1)
	defer ch.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		waitFallbackCtx  context.Context
		waitFallbackFunc context.CancelFunc
	)
	if !r.alwaysStandby && r.waitTime > 0 {
		waitFallbackCtx, waitFallbackFunc = context.WithCancel(ctx)
		defer waitFallbackFunc()
	}
	mainDNSCtx := dnsCtx.Clone()
	mainDNSCtx.SetID(mainDNSCtx.ID() + 1)
	mainDNSCtx.FlushColor()
	if r.mainWorkflow != nil {
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
			r.mainWorkflow,
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
			r.main,
		)
	}
	fallbackDNSCtx := dnsCtx.Clone()
	fallbackDNSCtx.SetID(fallbackDNSCtx.ID() + 2)
	fallbackDNSCtx.FlushColor()
	if r.fallbackWorkflow != nil {
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
			r.fallbackWorkflow,
			r.alwaysStandby,
			r.waitTime,
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
			r.fallback,
			r.alwaysStandby,
			r.waitTime,
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

type fallbackResult struct {
	isFallback bool
	dnsCtx     *adapter.DNSContext
	returnMode adapter.ReturnMode
}
