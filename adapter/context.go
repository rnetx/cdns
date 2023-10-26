package adapter

import (
	"context"
	"math"
	"math/rand"
	"net/netip"
	"time"

	"github.com/logrusorgru/aurora/v4"
	"github.com/miekg/dns"
)

func randomID() uint32 {
	start := uint32(math.Pow(10, 8))
	end := uint32(math.Pow(10, 9)) - 1
	diff := end - start
	return start + uint32(rand.Int63n(int64(diff)))
}

func idToColor(id uint32) aurora.Color {
	var color aurora.Color
	color = aurora.Color(uint8(id))
	color %= 215
	row := uint(color / 36)
	column := uint(color % 36)
	var r, g, b float32
	r = float32(row * 51)
	g = float32(column / 6 * 51)
	b = float32((column % 6) * 51)
	luma := 0.2126*r + 0.7152*g + 0.0722*b
	if luma < 60 {
		row = 5 - row
		column = 35 - column
		color = aurora.Color(row*36 + column)
	}
	color += 16
	color = color << 16
	color |= 1 << 14
	return color
}

var _ LogContext = (*DNSContext)(nil)

type DNSContext struct {
	ctx      context.Context
	initTime time.Time
	id       uint32
	color    aurora.Color
	//
	listener string
	clientIP netip.Addr
	req      *dns.Msg
	//
	resp            *dns.Msg
	respUpstreamTag string
	mark            uint64
	metadata        map[string]string
	//
	extraExchanges []ExtraExchange
	exchangeHooks  []ExchangeHook
}

func NewDNSContext(ctx context.Context, listener string, clientIP netip.Addr, req *dns.Msg) *DNSContext {
	c := &DNSContext{
		ctx:      ctx,
		initTime: time.Now(),
		id:       randomID(),
		listener: listener,
		clientIP: clientIP,
		req:      req,
	}
	return c
}

func (c *DNSContext) ID() uint32 {
	return c.id
}

func (c *DNSContext) SetID(id uint32) {
	c.id = id
}

func (c *DNSContext) Color() aurora.Color {
	if c.color == 0 {
		c.color = idToColor(c.id)
	}
	return c.color
}

func (c *DNSContext) FlushColor() {
	c.color = 0
}

func (c *DNSContext) Duration() time.Duration {
	return time.Since(c.initTime)
}

func (c *DNSContext) Context() context.Context {
	return c.ctx
}

func (c *DNSContext) Clone() *DNSContext {
	newDNSContext := &DNSContext{
		ctx:      c.ctx,
		initTime: c.initTime,
		id:       c.id,
		color:    c.color,
		listener: c.listener,
		clientIP: c.clientIP,
		req:      c.req.Copy(),
		mark:     c.mark,
	}
	if c.resp != nil {
		newDNSContext.resp = c.resp.Copy()
	}
	if c.metadata != nil && len(c.metadata) > 0 {
		newDNSContext.metadata = make(map[string]string)
		for k, v := range c.metadata {
			newDNSContext.metadata[k] = v
		}
	}
	if len(c.extraExchanges) > 0 {
		newDNSContext.extraExchanges = make([]ExtraExchange, 0, len(c.extraExchanges))
		for _, extraExchange := range c.extraExchanges {
			newExtraExchange := &ExtraExchange{
				Req: extraExchange.Req.Copy(),
			}
			if extraExchange.Resp != nil {
				newExtraExchange.Resp = extraExchange.Resp.Copy()
			}
			newDNSContext.extraExchanges = append(newDNSContext.extraExchanges, *newExtraExchange)
		}
	}
	if len(c.exchangeHooks) > 0 {
		newDNSContext.exchangeHooks = make([]ExchangeHook, 0, len(c.exchangeHooks))
		for _, hook := range c.exchangeHooks {
			newDNSContext.exchangeHooks = append(newDNSContext.exchangeHooks, hook.Clone())
		}
	}
	return newDNSContext
}

func (c *DNSContext) Listener() string {
	return c.listener
}

func (c *DNSContext) ClientIP() netip.Addr {
	return c.clientIP
}

func (c *DNSContext) ReqMsg() *dns.Msg {
	return c.req
}

func (c *DNSContext) RespMsg() *dns.Msg {
	return c.resp
}

func (c *DNSContext) SetRespMsg(resp *dns.Msg) {
	c.resp = resp
}

func (c *DNSContext) RespUpstreamTag() string {
	return c.respUpstreamTag
}

func (c *DNSContext) SetRespUpstreamTag(tag string) {
	c.respUpstreamTag = tag
}

func (c *DNSContext) Mark() uint64 {
	return c.mark
}

func (c *DNSContext) SetMark(mark uint64) {
	c.mark = mark
}

func (c *DNSContext) Metadata() map[string]string {
	if c.metadata == nil {
		c.metadata = make(map[string]string)
	}
	return c.metadata
}

type ExtraExchange struct {
	Req  *dns.Msg
	Resp *dns.Msg
}

func (c *DNSContext) ExtraExchanges() []ExtraExchange {
	return c.extraExchanges
}

func (c *DNSContext) SetExtraExchanges(extraExchanges []ExtraExchange) {
	c.extraExchanges = extraExchanges
}

func (c *DNSContext) AddExtraExchange(req *dns.Msg) {
	c.extraExchanges = append(c.extraExchanges, ExtraExchange{
		Req: req,
	})
}

func (c *DNSContext) FlushExchangeHooks() {
	newExchangeHooks := make([]ExchangeHook, 0, len(c.exchangeHooks))
	for _, hook := range c.exchangeHooks {
		if !hook.IsOnce() {
			newExchangeHooks = append(newExchangeHooks, hook)
		}
	}
	c.exchangeHooks = newExchangeHooks
}

func (c *DNSContext) ExchangeHooks() []ExchangeHook {
	return c.exchangeHooks
}

func (c *DNSContext) AddExchangeHook(hook ExchangeHook) {
	c.exchangeHooks = append(c.exchangeHooks, hook)
}
