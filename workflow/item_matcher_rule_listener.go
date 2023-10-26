package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"gopkg.in/yaml.v3"
)

var _ itemMatcherRule = (*itemMatcherListenerRule)(nil)

type itemMatcherListenerRule struct {
	listener []string
}

func (r *itemMatcherListenerRule) UnmarshalYAML(value *yaml.Node) error {
	var l utils.Listable[string]
	err := value.Decode(&l)
	if err != nil {
		return fmt.Errorf("listener: %w", err)
	}
	if len(l) == 0 {
		return fmt.Errorf("listener: missing listener")
	}
	r.listener = l
	return nil
}

func (r *itemMatcherListenerRule) check(_ context.Context, core adapter.Core) error {
	for _, l := range r.listener {
		if core.GetListener(l) == nil {
			return fmt.Errorf("listener: listener [%s] not found", l)
		}
	}
	return nil
}

func (r *itemMatcherListenerRule) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	ll := dnsCtx.Listener()
	for _, l := range r.listener {
		if ll == l {
			logger.DebugfContext(ctx, "listener: match listener: %s", l)
			return true, nil
		}
	}
	logger.DebugfContext(ctx, "listener: no match listener: %s", ll)
	return false, nil
}
