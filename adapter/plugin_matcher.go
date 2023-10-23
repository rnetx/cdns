package adapter

import "context"

type PluginMatcher interface {
	Tag() string
	Type() string
	LoadRunningArgs(ctx context.Context, argsID uint64, args any) error
	Match(ctx context.Context, dnsCtx *DNSContext, argsID uint64) (bool, error)
}
