package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"

	"gopkg.in/yaml.v3"
)

var _ itemMatcherRule = (*itemMatcherHasRespMsgRule)(nil)

type itemMatcherHasRespMsgRule struct {
	hasRespMsg bool
}

func (r *itemMatcherHasRespMsgRule) UnmarshalYAML(value *yaml.Node) error {
	var rr bool
	err := value.Decode(&rr)
	if err != nil {
		return fmt.Errorf("has-resp-msg: %w", err)
	}
	r.hasRespMsg = rr
	return nil
}

func (r *itemMatcherHasRespMsgRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemMatcherHasRespMsgRule) match(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (bool, error) {
	respMsg := dnsCtx.RespMsg()
	if r.hasRespMsg && respMsg != nil {
		logger.DebugfContext(ctx, "has-resp-msg: match has-resp-msg: true")
		return true, nil
	}
	if !r.hasRespMsg && respMsg == nil {
		logger.DebugfContext(ctx, "has-resp-msg: match has-resp-msg: false")
		return true, nil
	}
	logger.DebugfContext(ctx, "has-resp-msg: no match has-resp-msg: %t", respMsg != nil)
	return false, nil
}
