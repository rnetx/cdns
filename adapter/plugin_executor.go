package adapter

import "context"

type PluginExecutor interface {
	Tag() string
	Type() string
	LoadRunningArgs(ctx context.Context, argsID uint64, args any) error
	Exec(ctx context.Context, dnsCtx *DNSContext, argsID uint64) (ReturnMode, error)
}
