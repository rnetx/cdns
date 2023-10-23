package adapter

import (
	"context"

	"github.com/miekg/dns"
)

type ExchangeHook interface {
	BeforeExchange(ctx context.Context, dnsCtx *DNSContext, req *dns.Msg) (ReturnMode, error)               // Exchange Request
	AfterExchange(ctx context.Context, dnsCtx *DNSContext, req *dns.Msg, resp *dns.Msg) (ReturnMode, error) // Exchange Request
	IsOnce() bool
	Clone() ExchangeHook
}
