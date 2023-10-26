package workflow

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
)

var _ itemExecutorRule = (*itemExecutorUpstreamRule)(nil)

type itemExecutorUpstreamRule struct {
	upstreamTag string
	upstream    adapter.Upstream
}

func (r *itemExecutorUpstreamRule) UnmarshalYAML(value *yaml.Node) error {
	var u string
	err := value.Decode(&u)
	if err != nil {
		return fmt.Errorf("upstream: %w", err)
	}
	r.upstreamTag = u
	return nil
}

func (r *itemExecutorUpstreamRule) check(_ context.Context, core adapter.Core) error {
	u := core.GetUpstream(r.upstreamTag)
	if u == nil {
		return fmt.Errorf("upstream: upstream [%s] not found", r.upstreamTag)
	}
	r.upstream = u
	r.upstreamTag = "" // clean
	return nil
}

func (r *itemExecutorUpstreamRule) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	reqMsg := dnsCtx.ReqMsg()
	if reqMsg == nil {
		logger.DebugfContext(ctx, "upstream: request message is nil")
		return adapter.ReturnModeContinue, nil
	}
	exchangeHooks := dnsCtx.ExchangeHooks()
	defer dnsCtx.FlushExchangeHooks()
	for i, exchangeHook := range exchangeHooks {
		logger.DebugfContext(ctx, "upstream: exchange hook [%d]: before hook run", i)
		returnMode, err := exchangeHook.BeforeExchange(ctx, dnsCtx, reqMsg)
		if err != nil {
			logger.DebugfContext(ctx, "upstream: exchange hook [%d]: before hook run failed: %v", i, err)
			return adapter.ReturnModeUnknown, err
		}
		if returnMode != adapter.ReturnModeContinue {
			logger.DebugfContext(ctx, "upstream: exchange hook [%d]: before hook run: %s", i, returnMode.String())
			return returnMode, nil
		}
	}
	respMsg, err := r.upstream.Exchange(ctx, reqMsg)
	if err != nil {
		logger.DebugfContext(ctx, "upstream: upstream [%s] exchange failed: %v", r.upstream.Tag(), err)
		return adapter.ReturnModeUnknown, err
	}
	dnsCtx.SetRespMsg(respMsg)
	dnsCtx.SetRespUpstreamTag(r.upstream.Tag())
	extraExchanges := dnsCtx.ExtraExchanges()
	if len(extraExchanges) > 0 {
		ch := utils.NewSafeChan[exchangeResult](len(extraExchanges))
		defer ch.Close()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		for i, extraExchange := range extraExchanges {
			go func(ch *utils.SafeChan[exchangeResult], index int, reqMsg *dns.Msg) {
				respMsg, err := r.upstream.Exchange(ctx, reqMsg)
				if err != nil {
					select {
					case ch.SendChan() <- exchangeResult{
						index: index,
						req:   reqMsg,
						resp:  nil,
						err:   err,
					}:
					case <-ctx.Done():
					}
				} else {
					select {
					case ch.SendChan() <- exchangeResult{
						index: index,
						req:   reqMsg,
						resp:  respMsg,
					}:
					case <-ctx.Done():
					}
				}
			}(ch.Clone(), i, extraExchange.Req)
		}
		for i := 0; i < len(extraExchanges); i++ {
			select {
			case res := <-ch.ReceiveChan():
				if res.err == nil {
					extraExchanges[res.index].Resp = res.resp
				}
			case <-ctx.Done():
			}
		}
		dnsCtx.SetExtraExchanges(extraExchanges)
	}
	for i, exchangeHook := range exchangeHooks {
		logger.DebugfContext(ctx, "upstream: exchange hook [%d]: after hook run", i)
		returnMode, err := exchangeHook.AfterExchange(ctx, dnsCtx, reqMsg, respMsg)
		if err != nil {
			logger.DebugfContext(ctx, "upstream: exchange hook [%d]: after hook run failed: %v", i, err)
			return adapter.ReturnModeUnknown, err
		}
		if returnMode != adapter.ReturnModeContinue {
			logger.DebugfContext(ctx, "upstream: exchange hook [%d]: after hook run: %s", i, returnMode.String())
			return returnMode, nil
		}
	}
	return adapter.ReturnModeContinue, nil
}

type exchangeResult struct {
	index int
	req   *dns.Msg
	resp  *dns.Msg
	err   error
}
