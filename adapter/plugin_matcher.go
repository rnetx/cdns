package adapter

import "context"

type PluginMatcher interface {
	Tag() string
	Type() string
	LoadRunningArgs(ctx context.Context, args any) (uint16, error)
	Match(ctx context.Context, dnsCtx *DNSContext, argsID uint16) (bool, error)
}
