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

const (
	upstreamStrategyPreferIPv4 = "prefer-ipv4"
	upstreamStrategyPreferIPv6 = "prefer-ipv6"
)

type itemExecutorUpstreamRule struct {
	upstreamTag string
	upstream    adapter.Upstream
	strategy    string
}

type itemExecutorUpstreamRuleOptions struct {
	Tag      string `yaml:"tag"`
	Strategy string `yaml:"strategy"`
}

func (r *itemExecutorUpstreamRule) UnmarshalYAML(value *yaml.Node) error {
	var u string
	err := value.Decode(&u)
	if err == nil {
		r.upstreamTag = u
		return nil
	}
	var o itemExecutorUpstreamRuleOptions
	err2 := value.Decode(&o)
	if err2 != nil {
		return fmt.Errorf("upstream: %w | %w", err, err2)
	}
	if o.Tag == "" {
		return fmt.Errorf("upstream: missing tag")
	}
	r.upstreamTag = o.Tag
	switch o.Strategy {
	case "":
		return fmt.Errorf("upstream: missing strategy")
	case upstreamStrategyPreferIPv4, "a", "A", "4", "IPv4", "IP4", "ip4", "ipv4":
		r.strategy = upstreamStrategyPreferIPv4
	case upstreamStrategyPreferIPv6, "aaaa", "AAAA", "6", "IPv6", "IP6", "ip6", "ipv6":
		r.strategy = upstreamStrategyPreferIPv6
	default:
		return fmt.Errorf("upstream: invalid strategy: %s", o.Strategy)
	}
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
	question := reqMsg.Question[0]
	qType := question.Qtype
	if (qType != dns.TypeA && qType != dns.TypeAAAA) || r.strategy == "" || (qType == dns.TypeA && r.strategy == upstreamStrategyPreferIPv4) || (qType == dns.TypeAAAA && r.strategy == upstreamStrategyPreferIPv6) {
		respMsg, err := r.upstream.Exchange(ctx, reqMsg)
		if err != nil {
			logger.DebugfContext(ctx, "upstream: upstream [%s] exchange failed: %v", r.upstream.Tag(), err)
			return adapter.ReturnModeUnknown, err
		}
		dnsCtx.SetRespMsg(respMsg)
		dnsCtx.SetRespUpstreamTag(r.upstream.Tag())
		return adapter.ReturnModeContinue, nil
	}
	var extraReqMsg *dns.Msg
	if qType == dns.TypeA {
		extraReqMsg = &dns.Msg{}
		extraReqMsg.SetQuestion(question.Name, dns.TypeAAAA)
	}
	if qType == dns.TypeAAAA {
		extraReqMsg = &dns.Msg{}
		extraReqMsg.SetQuestion(question.Name, dns.TypeA)
	}
	ch := utils.NewSafeChan[exchangeResult](2)
	defer ch.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for i := 0; i < 2; i++ {
		var req *dns.Msg
		if i == 0 {
			req = reqMsg
		} else {
			req = extraReqMsg
		}
		go func(ctx context.Context, ch *utils.SafeChan[exchangeResult], req *dns.Msg) {
			defer ch.Close()
			respMsg, err := r.upstream.Exchange(ctx, req)
			if err != nil {
				logger.DebugfContext(ctx, "upstream: upstream [%s] exchange failed: %v", r.upstream.Tag(), err)
				select {
				case ch.SendChan() <- exchangeResult{req: req, isErr: true}:
				default:
				}
			} else {
				select {
				case ch.SendChan() <- exchangeResult{req: req, resp: respMsg}:
				default:
				}
			}
		}(ctx, ch.Clone(), req)
	}
	var (
		respMsg      *dns.Msg
		extraRespMsg *dns.Msg
	)
	for i := 0; i < 2; i++ {
		select {
		case result := <-ch.ReceiveChan():
			if result.isErr && result.req == reqMsg {
				return adapter.ReturnModeUnknown, fmt.Errorf("upstream: upstream [%s] exchange failed", r.upstream.Tag())
			}
			if result.req == reqMsg {
				respMsg = result.resp
			} else {
				extraRespMsg = result.resp
			}
		case <-ctx.Done():
			logger.DebugfContext(ctx, "upstream: context done")
			return adapter.ReturnModeUnknown, ctx.Err()
		}
	}
	if extraRespMsg == nil {
		return adapter.ReturnModeUnknown, fmt.Errorf("upstream: upstream [%s] prefer extra request exchange failed", r.upstream.Tag())
	}
	var tag bool
	for _, rr := range extraRespMsg.Answer {
		if rr.Header().Rrtype == qType {
			tag = true
			break
		}
	}
	if !tag {
		logger.DebugfContext(ctx, "upstream: qType: %s, strategy: %s", dns.TypeToString[qType], r.strategy)
	} else {
		logger.DebugfContext(ctx, "upstream: qType: %s, but strategy: %s, drop response and generate empty response", dns.TypeToString[qType], r.strategy)
		respMsg = &dns.Msg{}
		respMsg.SetReply(reqMsg)
		respMsg.Ns = []dns.RR{utils.FakeSOA(question.Name)}
	}
	dnsCtx.SetRespMsg(respMsg)
	dnsCtx.SetRespUpstreamTag(r.upstream.Tag())
	return adapter.ReturnModeContinue, nil
}

type exchangeResult struct {
	req   *dns.Msg
	resp  *dns.Msg
	isErr bool
}
