package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"gopkg.in/yaml.v3"
)

var _ itemMatcherRule = (*itemMatcherMarkRule)(nil)

type itemMatcherMarkRule struct {
	mark []uint64
}

func (r *itemMatcherMarkRule) UnmarshalYAML(value *yaml.Node) error {
	var m utils.Listable[uint64]
	err := value.Decode(&m)
	if err != nil {
		return fmt.Errorf("mark: %w", err)
	}
	if len(m) == 0 {
		return fmt.Errorf("mark: missing mark")
	}
	r.mark = m
	return nil
}

func (r *itemMatcherMarkRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemMatcherMarkRule) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	mark := dnsCtx.Mark()
	for _, m := range r.mark {
		if m == mark {
			logger.DebugfContext(ctx, "mark: match mark: %d", m)
			return true, nil
		}
	}
	logger.DebugfContext(ctx, "mark: no match mark: %d", mark)
	return false, nil
}
