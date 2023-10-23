package adapter

import (
	"context"

	"github.com/miekg/dns"
)

type Upstream interface {
	Tag() string
	Type() string
	Dependencies() []string
	Exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error)
}
