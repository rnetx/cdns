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
	waitTime            time.Duration
}

type itemExecutorFallbackRuleOptions struct {
	Main             utils.Listable[RuleOptions] `yaml:"main,omitempty"`
	Fallback         utils.Listable[RuleOptions] `yaml:"fallback,omitempty"`
	MainWorkflow     string                      `yaml:"main-workflow,omitempty"`
	FallbackWorkflow string                      `yaml:"fallback-workflow,omitempty"`
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
	r.waitTime = time.Duration(o.WaitTime)
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
	ch := utils.NewSafeChan[fallbackResult](2)
	defer ch.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		waitFallbackCtx  context.Context
		waitFallbackFunc context.CancelFunc
	)
	if r.waitTime > 0 {
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
				select {
				case ch.SendChan() <- fallbackResult{
					err: err,
				}:
				default:
				}
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
					select {
					case ch.SendChan() <- fallbackResult{
						err: err,
					}:
					default:
					}
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
			waitFallbackCtx context.Context,
		) {
			defer ch.Close()
			logger.DebugfContext(ctx, "fallback: fallback workflow [%s] exec, id: %d", w.Tag(), dnsCtx.ID())
			if waitFallbackCtx != nil {
				logger.DebugfContext(ctx, "fallback: fallback workflow [%s] waiting...", w.Tag())
				<-waitFallbackCtx.Done()
				select {
				case <-ctx.Done():
					select {
					case ch.SendChan() <- fallbackResult{
						err:        ctx.Err(),
						isFallback: true,
					}:
					default:
					}
					return
				default:
				}
			}
			returnMode, err := w.Exec(adapter.SaveLogContext(ctx, dnsCtx), dnsCtx)
			if err != nil {
				logger.DebugfContext(ctx, "fallback: fallback workflow [%s] exec failed: %v", w.Tag(), err)
				select {
				case ch.SendChan() <- fallbackResult{
					err:        err,
					isFallback: true,
				}:
				default:
				}
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
			waitFallbackCtx context.Context,
		) {
			defer ch.Close()
			logger.DebugfContext(ctx, "fallback: fallback rules exec, id: %d", dnsCtx.ID())
			if waitFallbackCtx != nil {
				logger.DebugContext(ctx, "fallback: fallback rules waiting...")
				<-waitFallbackCtx.Done()
				select {
				case <-ctx.Done():
					select {
					case ch.SendChan() <- fallbackResult{
						err:        ctx.Err(),
						isFallback: true,
					}:
					default:
					}
					return
				default:
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
					select {
					case ch.SendChan() <- fallbackResult{
						err:        err,
						isFallback: true,
					}:
					default:
					}
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
			waitFallbackCtx,
		)
	}
	var (
		mainErr        error
		fallbackResult fallbackResult
	)
	for i := 0; i < 2; i++ {
		select {
		case result := <-ch.ReceiveChan():
			if !result.isFallback && result.err == nil {
				logger.DebugfContext(ctx, "fallback: main exec success: %s", result.returnMode.String())
				oldID := dnsCtx.ID()
				*dnsCtx = *result.dnsCtx
				dnsCtx.SetID(oldID)
				dnsCtx.FlushColor()
				return result.returnMode, nil
			}
			if !result.isFallback {
				mainErr = result.err
			} else {
				fallbackResult = result
			}
		case <-ctx.Done():
			logger.DebugfContext(ctx, "fallback: timeout")
			return adapter.ReturnModeUnknown, ctx.Err()
		}
	}
	if fallbackResult.err == nil {
		logger.DebugfContext(ctx, "fallback: main exec failed: %s | fallback exec success: %s", mainErr, fallbackResult.returnMode.String())
		oldID := dnsCtx.ID()
		*dnsCtx = *fallbackResult.dnsCtx
		dnsCtx.SetID(oldID)
		dnsCtx.FlushColor()
		return fallbackResult.returnMode, nil
	}
	err := fmt.Errorf("fallback: main exec failed: %s | fallback exec failed: %s", mainErr, fallbackResult.err)
	logger.ErrorContext(ctx, err)
	return adapter.ReturnModeUnknown, err
}

type fallbackResult struct {
	isFallback bool
	dnsCtx     *adapter.DNSContext
	returnMode adapter.ReturnMode
	err        error
}
