package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"gopkg.in/yaml.v3"
)

var _ itemExecutorRule = (*itemExecutorJumpToRule)(nil)

type itemExecutorJumpToRule struct {
	jumpToTag []string
	jumpTo    []adapter.Workflow
}

func (r *itemExecutorJumpToRule) UnmarshalYAML(value *yaml.Node) error {
	var j utils.Listable[string]
	err := value.Decode(&j)
	if err != nil {
		return fmt.Errorf("jump-to: %w", err)
	}
	if len(j) == 0 {
		return fmt.Errorf("jump-to: missing jump-to")
	}
	r.jumpToTag = j
	return nil
}

func (r *itemExecutorJumpToRule) check(_ context.Context, core adapter.Core) error {
	r.jumpTo = make([]adapter.Workflow, 0, len(r.jumpToTag))
	for _, tag := range r.jumpToTag {
		w := core.GetWorkflow(tag)
		if w == nil {
			return fmt.Errorf("jump-to: workflow [%s] not found", tag)
		}
		r.jumpTo = append(r.jumpTo, w)
	}
	r.jumpToTag = nil // clean
	return nil
}

func (r *itemExecutorJumpToRule) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	for _, w := range r.jumpTo {
		logger.DebugfContext(ctx, "jump-to: jump to workflow [%s]", w.Tag())
		returnMode, err := w.Exec(ctx, dnsCtx)
		if err != nil {
			logger.ErrorfContext(ctx, "jump-to: workflow [%s] exec failed: %v", w.Tag(), err)
			return adapter.ReturnModeUnknown, err
		}
		logger.DebugfContext(ctx, "jump-to: workflow [%s]: %s", w.Tag(), returnMode.String())
		if returnMode != adapter.ReturnModeContinue {
			return returnMode, nil
		}
	}
	return adapter.ReturnModeContinue, nil
}
