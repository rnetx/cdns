package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

var _ itemExecutorRule = (*itemExecutorMetadataRule)(nil)

type itemExecutorMetadataRule struct {
	metadata map[string]string
}

func (r *itemExecutorMetadataRule) UnmarshalYAML(value *yaml.Node) error {
	var m map[string]string
	err := value.Decode(&m)
	if err != nil {
		return fmt.Errorf("metadata: %w", err)
	}
	r.metadata = m
	return nil
}

func (r *itemExecutorMetadataRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemExecutorMetadataRule) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	metadata := dnsCtx.Metadata()
	for k, v := range r.metadata {
		if v == "" {
			logger.DebugfContext(ctx, "metadata: delete metadata: %s", k)
			delete(metadata, k)
		} else {
			vv, ok := metadata[k]
			if ok {
				logger.DebugfContext(ctx, "metadata: set metadata: key: %s, value: %s => %s", k, vv, v)
			} else {
				logger.DebugfContext(ctx, "metadata: set metadata: key: %s, value: %s", k, v)
			}
			metadata[k] = v
		}
	}
	return adapter.ReturnModeContinue, nil
}
