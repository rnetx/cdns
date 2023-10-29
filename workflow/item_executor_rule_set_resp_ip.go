package workflow

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
)

var _ itemExecutorRule = (*itemExecutorSetRespIPRule)(nil)

type itemExecutorSetRespIPRule struct {
	ipv4 bool
	ipv6 bool
	addr []netip.Prefix
}

func (r *itemExecutorSetRespIPRule) UnmarshalYAML(value *yaml.Node) error {
	var i utils.Listable[string]
	err := value.Decode(&i)
	if err != nil {
		return fmt.Errorf("set-resp-ip: %w", err)
	}
	if len(i) == 0 {
		return fmt.Errorf("set-resp-ip: ip is empty")
	}
	r.addr = make([]netip.Prefix, 0, len(i))
	for _, s := range i {
		ip, err := netip.ParseAddr(s)
		if err == nil {
			bits := 0
			if ip.Is4() {
				bits = 32
				r.ipv4 = true
			} else {
				bits = 128
				r.ipv6 = true
			}
			r.addr = append(r.addr, netip.PrefixFrom(ip, bits))
			continue
		}
		prefix, err := netip.ParsePrefix(s)
		if err == nil {
			if prefix.Addr().Is4() {
				r.ipv4 = true
			} else {
				r.ipv6 = true
			}
			r.addr = append(r.addr, prefix)
			continue
		}
		return fmt.Errorf("set-resp-ip: invalid ip: %s, error: %w", s, err)
	}
	return nil
}

func (r *itemExecutorSetRespIPRule) check(_ context.Context, _ adapter.Core) error {
	return nil
}

func (r *itemExecutorSetRespIPRule) exec(ctx context.Context, core adapter.Core, logger log.Logger, dnsCtx *adapter.DNSContext) (adapter.ReturnMode, error) {
	reqMsg := dnsCtx.ReqMsg()
	if reqMsg == nil {
		return adapter.ReturnModeContinue, fmt.Errorf("set-resp-ip: request message is nil")
	}
	question := reqMsg.Question[0]
	qName := question.Name
	qType := question.Qtype
	if qType == dns.TypeA && !r.ipv4 {
		return adapter.ReturnModeContinue, fmt.Errorf("set-resp-ip: request type is A, but no ipv4 ip")
	}
	if qType == dns.TypeAAAA && !r.ipv6 {
		return adapter.ReturnModeContinue, fmt.Errorf("set-resp-ip: request type is AAAA, but no ipv6 ip")
	}
	answers := make([]dns.RR, 0, len(r.addr))
	for _, addr := range r.addr {
		ip := addr.Addr()
		if (addr.Bits() == 32 && ip.Is4()) || (addr.Bits() == 128 && ip.Is6()) {
			ip = utils.RandomAddrFromPrefix(addr)
		}
		if qType == dns.TypeA && ip.Is4() {
			answers = append(answers, &dns.A{
				Hdr: dns.RR_Header{
					Name:   qName,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    600,
				},
				A: ip.AsSlice(),
			})
		}
		if qType == dns.TypeAAAA && ip.Is6() {
			answers = append(answers, &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   qName,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    600,
				},
				AAAA: ip.AsSlice(),
			})
		}
	}
	respMsg := &dns.Msg{}
	respMsg.SetReply(reqMsg)
	respMsg.Answer = answers
	dnsCtx.SetRespMsg(respMsg)
	return adapter.ReturnModeContinue, nil
}
