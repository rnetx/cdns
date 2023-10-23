package adapter

import (
	"context"
	"net/netip"

	"github.com/miekg/dns"
)

type Listener interface {
	Tag() string
	Type() string
	Handle(ctx context.Context, req *dns.Msg, clientAddr netip.AddrPort) *dns.Msg
}
