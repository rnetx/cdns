package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

var _ itemExecutorRule = (*itemExecutorMarkRule)(nil)

type itemExecutorMarkRule struct {
	mark uint64
}

func (r *itemExecutorMarkRule) UnmarshalYAML(value *yaml.Node) error {
	var m uint64
	err := value.Decode(&m)
	if err != nil {
		return fmt.Errorf("mark: %w", err)
	}
	r.mark = m
	return nil
}

func (r *itemExecutorMarkRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemExecutorMarkRule) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	mark := dnsCtx.Mark()
	dnsCtx.SetMark(r.mark)
	logger.DebugfContext(ctx, "mark: set mark: %d => %d", mark, r.mark)
	return adapter.ReturnModeContinue, nil
}
