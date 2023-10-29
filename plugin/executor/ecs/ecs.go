package ecs

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"
	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
)

const Type = "ecs"

func init() {
	plugin.RegisterPluginExecutor(Type, NewECS)
}

const (
	DefaultMask4 = 24
	DefaultMask6 = 60
)

type Args struct {
	IPv4        netip.Addr `json:"ipv4"`
	IPv6        netip.Addr `json:"ipv6"`
	Mask4       uint8      `json:"mask4"`
	Mask6       uint8      `json:"mask6"`
	UseClientIP bool       `json:"use-client-ip"`
}

var _ adapter.PluginExecutor = (*ECS)(nil)

type ECS struct {
	tag    string
	logger log.Logger

	ipv4        netip.Addr
	ipv6        netip.Addr
	mask4       uint8
	mask6       uint8
	useClientIP bool
}

func NewECS(_ context.Context, _ adapter.Core, logger log.Logger, tag string, args any) (adapter.PluginExecutor, error) {
	e := &ECS{
		tag:    tag,
		logger: logger,
	}
	var a Args
	err := utils.JsonDecode(args, &a)
	if err != nil {
		return nil, fmt.Errorf("parse args failed: %w", err)
	}
	e.useClientIP = a.UseClientIP
	if a.Mask4 == 0 {
		e.mask4 = DefaultMask4
	} else if a.Mask4 > 32 {
		return nil, fmt.Errorf("invalid mask4: %d", a.Mask4)
	} else {
		e.mask4 = a.Mask4
	}
	if a.Mask6 == 0 {
		e.mask6 = DefaultMask6
	} else if a.Mask6 > 128 {
		return nil, fmt.Errorf("invalid mask6: %d", a.Mask6)
	} else {
		e.mask6 = a.Mask6
	}
	if !a.UseClientIP {
		if !a.IPv4.IsValid() {
			return nil, fmt.Errorf("invalid ipv4: %s", a.IPv4)
		}
		if !a.IPv6.IsValid() {
			return nil, fmt.Errorf("invalid ipv6: %s", a.IPv6)
		}
		e.ipv4 = netip.PrefixFrom(a.IPv4, int(e.mask4)).Masked().Addr()
		e.ipv6 = netip.PrefixFrom(a.IPv6, int(e.mask6)).Masked().Addr()
	}
	return e, nil
}

func (e *ECS) Tag() string {
	return e.tag
}

func (e *ECS) Type() string {
	return Type
}

func (e *ECS) LoadRunningArgs(_ context.Context, _ any) (uint16, error) {
	return 0, nil
}

func (e *ECS) addECS(dnsCtx *adapter.DNSContext, req *dns.Msg) string {
	for _, rr := range req.Extra {
		if opt, ok := rr.(*dns.OPT); ok {
			for _, o := range opt.Option {
				if o.Option() == dns.EDNS0SUBNET {
					return ""
				}
			}
		}
	}
	if req.Question[0].Qclass != dns.ClassINET {
		return ""
	}

	edns0Subnet := new(dns.EDNS0_SUBNET)
	clientIP := dnsCtx.ClientIP()
	s := ""
	if e.useClientIP {
		if clientIP.Is4() {
			edns0Subnet.Family = 1
			edns0Subnet.SourceNetmask = e.mask4
		} else {
			edns0Subnet.Family = 2
			edns0Subnet.SourceNetmask = e.mask6
		}
		edns0Subnet.Address = clientIP.AsSlice()
		s = netip.PrefixFrom(clientIP, int(edns0Subnet.SourceNetmask)).String()
	} else {
		if clientIP.Is4() {
			edns0Subnet.Family = 1
			edns0Subnet.SourceNetmask = e.mask4
			edns0Subnet.Address = e.ipv4.AsSlice()
			s = netip.PrefixFrom(e.ipv4, int(edns0Subnet.SourceNetmask)).String()
		} else {
			edns0Subnet.Family = 2
			edns0Subnet.SourceNetmask = e.mask6
			edns0Subnet.Address = e.ipv6.AsSlice()
			s = netip.PrefixFrom(e.ipv6, int(edns0Subnet.SourceNetmask)).String()
		}
	}
	edns0Subnet.SourceScope = 0
	edns0Subnet.Code = dns.EDNS0SUBNET

	opt := new(dns.OPT)
	opt.Hdr.Name = "."
	opt.Hdr.Rrtype = dns.TypeOPT
	opt.SetUDPSize(dns.DefaultMsgSize)
	opt.Option = append(opt.Option, edns0Subnet)
	req.Extra = append(req.Extra, opt)

	return s
}

func (e *ECS) Exec(ctx context.Context, dnsCtx *adapter.DNSContext, _ uint16) (adapter.ReturnMode, error) {
	req := dnsCtx.ReqMsg()
	if req == nil {
		e.logger.DebugContext(ctx, "request message is nil")
		return adapter.ReturnModeContinue, nil
	}
	s := e.addECS(dnsCtx, req)
	if s != "" {
		e.logger.DebugfContext(ctx, "add ecs: %s", s)
	}
	return adapter.ReturnModeContinue, nil
}
