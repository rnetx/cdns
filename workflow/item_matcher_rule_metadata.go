package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

var _ itemMatcherRule = (*itemMatcherMetadataRule)(nil)

type itemMatcherMetadataRule struct {
	metadata map[string]string
}

func (r *itemMatcherMetadataRule) UnmarshalYAML(value *yaml.Node) error {
	var m map[string]string
	err := value.Decode(&m)
	if err != nil {
		return fmt.Errorf("metadata: %w", err)
	}
	if len(m) == 0 {
		return fmt.Errorf("metadata: missing metadata")
	}
	r.metadata = m
	return nil
}

func (r *itemMatcherMetadataRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemMatcherMetadataRule) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	metadata := dnsCtx.Metadata()
	match := true
	for k1, v1 := range r.metadata {
		v2, ok := metadata[k1]
		if !ok || v2 != v1 {
			logger.DebugfContext(ctx, "metadata: no match metadata: %s => %s, value: %s", k1, v1, v2)
			match = false
			break
		}
	}
	if match {
		logger.DebugContext(ctx, "metadata: match metadata")
		return true, nil
	} else {
		logger.DebugContext(ctx, "metadata: no match metadata")
		return false, nil
	}
}
