package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"gopkg.in/yaml.v3"
)

var _ itemExecutorRule = (*itemExecutorParallelRule)(nil)

type itemExecutorParallelRule struct {
	workflowTags []string
	workflows    []adapter.Workflow
}

type itemExecutorParallelRuleOptions struct {
	Workflows utils.Listable[string] `yaml:"workflows,omitempty"`
}

func (r *itemExecutorParallelRule) UnmarshalYAML(value *yaml.Node) error {
	var o itemExecutorParallelRuleOptions
	err := value.Decode(&o)
	if err != nil {
		return fmt.Errorf("parallel: %w", err)
	}
	r.workflowTags = o.Workflows
	return nil
}

func (r *itemExecutorParallelRule) check(ctx context.Context, core adapter.Core) error {
	r.workflows = make([]adapter.Workflow, 0, len(r.workflowTags))
	for _, tag := range r.workflowTags {
		w := core.GetWorkflow(tag)
		if w == nil {
			return fmt.Errorf("parallel: workflow [%s] not found", tag)
		}
		r.workflows = append(r.workflows, w)
	}
	r.workflowTags = nil // clean
	return nil
}

func (r *itemExecutorParallelRule) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	ch := utils.NewSafeChan[parallelResult](1)
	defer ch.Close()
	for i, w := range r.workflows {
		iDNSCtx := dnsCtx.Clone()
		iDNSCtx.SetID(iDNSCtx.ID() + uint32(i) + 1)
		iDNSCtx.FlushColor()
		logger.DebugfContext(ctx, "parallel: workflow [%s] exec, id: %d", w.Tag(), iDNSCtx.ID())
		go func(
			ctx context.Context,
			dnsCtx *adapter.DNSContext,
			ch *utils.SafeChan[parallelResult],
			w adapter.Workflow,
		) {
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
			} else {
				select {
				case ch.SendChan() <- parallelResult{
					w:   w,
					err: err,
				}:
				default:
				}
			}
		}(
			ctx,
			iDNSCtx,
			ch.Clone(),
			w,
		)
	}
	for i := 0; i < len(r.workflows); i++ {
		select {
		case result := <-ch.ReceiveChan():
			if result.err == nil {
				logger.DebugfContext(ctx, "parallel: workflow [%s] exec success: %s", result.w.Tag(), result.returnMode.String())
				oldID := dnsCtx.ID()
				*dnsCtx = *result.dnsCtx
				dnsCtx.SetID(oldID)
				dnsCtx.FlushColor()
				return result.returnMode, nil
			}
		case <-ctx.Done():
			logger.ErrorContext(ctx, "parallel: timeout")
			return adapter.ReturnModeUnknown, ctx.Err()
		}
	}
	err := fmt.Errorf("parallel: all workflow exec failed")
	logger.ErrorContext(ctx, err)
	return adapter.ReturnModeUnknown, err
}

type parallelResult struct {
	w          adapter.Workflow
	dnsCtx     *adapter.DNSContext
	returnMode adapter.ReturnMode
	err        error
}
