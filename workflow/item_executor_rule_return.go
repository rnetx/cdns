package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
)

var _ itemExecutorRule = (*itemExecutorReturnRule)(nil)

type itemExecutorReturnRule struct {
	_return string
}

func (r *itemExecutorReturnRule) UnmarshalYAML(value *yaml.Node) error {
	var re any
	err := value.Decode(&re)
	if err != nil {
		return fmt.Errorf("return: %w", err)
	}
	switch rr := re.(type) {
	case string:
		rr = strings.ToLower(rr)
		switch rr {
		case "all":
			r._return = "all"
		case "once":
			r._return = "once"
		case "success":
			r._return = "success" // all
		case "failure", "fail":
			r._return = "failure" // all
		case "nxdomain":
			r._return = "nxdomain" // all
		case "refused":
			r._return = "refused" // all
		default:
			return fmt.Errorf("return: invalid return: %s", rr)
		}
	case bool:
		if rr {
			r._return = "all"
		}
	default:
		return fmt.Errorf("return: invalid return: %v", rr)
	}
	return nil
}

func (r *itemExecutorReturnRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemExecutorReturnRule) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	var rcode int
	switch r._return {
	case "all":
		logger.DebugContext(ctx, "return: return all")
		return adapter.ReturnModeReturnAll, nil
	case "once":
		logger.DebugContext(ctx, "return: return once")
		return adapter.ReturnModeReturnOnce, nil
	case "success":
		logger.DebugContext(ctx, "return: return success")
		rcode = dns.RcodeSuccess
	case "failure":
		logger.DebugContext(ctx, "return: return failure")
		rcode = dns.RcodeServerFailure
	case "nxdomain":
		logger.DebugContext(ctx, "return: return nxdomain")
		rcode = dns.RcodeNameError
	case "refused":
		logger.DebugContext(ctx, "return: return refused")
		rcode = dns.RcodeRefused
	case "":
		return adapter.ReturnModeContinue, nil
	}
	var name string
	question := dnsCtx.ReqMsg().Question
	if len(question) > 0 {
		name = question[0].Name
	}
	newRespMsg := &dns.Msg{}
	newRespMsg.SetRcode(dnsCtx.ReqMsg(), rcode)
	newRespMsg.Ns = []dns.RR{utils.FakeSOA(name)}
	dnsCtx.SetRespMsg(newRespMsg)
	return adapter.ReturnModeReturnAll, nil
}
