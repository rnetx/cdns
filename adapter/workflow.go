package adapter

import "context"

type Workflow interface {
	Tag() string
	Check() error
	Exec(ctx context.Context, dnsCtx *DNSContext) (ReturnMode, error)
}
