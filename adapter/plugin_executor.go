package adapter

import "context"

type PluginExecutor interface {
	Tag() string
	Type() string
	LoadRunningArgs(ctx context.Context, args any) (uint16, error)
	Exec(ctx context.Context, dnsCtx *DNSContext, argsID uint16) (ReturnMode, error)
}
