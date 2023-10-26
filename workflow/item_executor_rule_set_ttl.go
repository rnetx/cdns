package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

var _ itemExecutorRule = (*itemExecutorSetTTLRule)(nil)

type itemExecutorSetTTLRule struct {
	ttl uint32
}

func (r *itemExecutorSetTTLRule) UnmarshalYAML(value *yaml.Node) error {
	var s uint32
	err := value.Decode(&s)
	if err != nil {
		return fmt.Errorf("set-ttl: %w", err)
	}
	r.ttl = s
	return nil
}

func (r *itemExecutorSetTTLRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemExecutorSetTTLRule) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	respMsg := dnsCtx.RespMsg()
	if respMsg == nil {
		logger.DebugfContext(ctx, "set-ttl: response message is nil")
		return adapter.ReturnModeContinue, nil
	}
	for i := range respMsg.Answer {
		respMsg.Answer[i].Header().Ttl = r.ttl
	}
	for i := range respMsg.Ns {
		respMsg.Ns[i].Header().Ttl = r.ttl
	}
	for i := range respMsg.Extra {
		respMsg.Extra[i].Header().Ttl = r.ttl
	}
	logger.DebugfContext(ctx, "set-ttl: %d", r.ttl)
	return adapter.ReturnModeContinue, nil
}
