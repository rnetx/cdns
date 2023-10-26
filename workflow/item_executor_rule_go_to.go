package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

var _ itemExecutorRule = (*itemExecutorGoToRule)(nil)

type itemExecutorGoToRule struct {
	goToTag string
	goTo    adapter.Workflow
}

func (r *itemExecutorGoToRule) UnmarshalYAML(value *yaml.Node) error {
	var g string
	err := value.Decode(&g)
	if err != nil {
		return fmt.Errorf("go-to: %w", err)
	}
	if g == "" {
		return fmt.Errorf("go-to: missing go-to")
	}
	r.goToTag = g
	return nil
}

func (r *itemExecutorGoToRule) check(_ context.Context, core adapter.Core) error {
	w := core.GetWorkflow(r.goToTag)
	if w == nil {
		return fmt.Errorf("go-to: workflow [%s] not found", r.goToTag)
	}
	r.goTo = w
	r.goToTag = "" // clean
	return nil
}

func (r *itemExecutorGoToRule) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	logger.DebugfContext(ctx, "go-to: go to workflow [%s]", r.goTo.Tag())
	return r.goTo.Exec(ctx, dnsCtx)
}
