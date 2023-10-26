package workflow

import (
	"context"
	"fmt"
	"os"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

var _ itemMatcherRule = (*itemMatcherEnvRule)(nil)

type itemMatcherEnvRule struct {
	env map[string]string
}

func (r *itemMatcherEnvRule) UnmarshalYAML(value *yaml.Node) error {
	var e map[string]string
	err := value.Decode(&e)
	if err != nil {
		return fmt.Errorf("env: %w", err)
	}
	if len(e) == 0 {
		return fmt.Errorf("env: missing env")
	}
	r.env = e
	return nil
}

func (r *itemMatcherEnvRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemMatcherEnvRule) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	for k, v := range r.env {
		if os.Getenv(k) == v {
			logger.DebugfContext(ctx, "env: match env: %s => %s", k, v)
			return true, nil
		}
	}
	logger.DebugContext(ctx, "env: no match env")
	return false, nil
}
