package ecs

import (
	"github.com/rnetx/cdns/log"

	"github.com/miekg/dns"
)

const Type = "ecs"

type Args struct {
	IPv4  string `json:"ipv4"`
	IPv6  string `json:"ipv6"`
	Mask4 uint8  `json:"mask4"`
	Mask6 uint8  `json:"mask6"`
}

type ECS struct {
	tag    string
	logger log.Logger
}

func (e *ECS) addECS(req *dns.Msg) {
	for _, rr := range req.Extra {
		if opt, ok := rr.(*dns.OPT); ok {
			for _, o := range opt.Option {
				if o.Option() == dns.EDNS0SUBNET {
					return
				}
			}
		}
	}
	if req.Question[0].Qclass != dns.ClassINET {
		return
	}
}
