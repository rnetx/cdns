package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

var _ itemExecutorRule = (*itemExecutorCleanRule)(nil)

type itemExecutorCleanRule struct {
	clean bool
}

func (r *itemExecutorCleanRule) UnmarshalYAML(value *yaml.Node) error {
	var c bool
	err := value.Decode(&c)
	if err != nil {
		return fmt.Errorf("clean: %w", err)
	}
	r.clean = c
	return nil
}

func (r *itemExecutorCleanRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemExecutorCleanRule) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	if r.clean {
		dnsCtx.SetRespMsg(nil)
		logger.DebugContext(ctx, "clean: clean response message")
	}
	return adapter.ReturnModeContinue, nil
}
