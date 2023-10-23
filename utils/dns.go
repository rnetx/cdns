package utils

import "github.com/miekg/dns"

// from mosdns(https://github.com/IrineSistiana/mosdns), thank for @IrineSistiana
func FakeSOA(name string) *dns.SOA {
	if name == "" {
		name = "."
	}
	return &dns.SOA{
		Hdr: dns.RR_Header{
			Name:   name,
			Rrtype: dns.TypeSOA,
			Class:  dns.ClassINET,
			Ttl:    10,
		},
		Ns:      "fake-ns.cdns.",
		Mbox:    "fake-mbox.cdns.",
		Serial:  2023060700,
		Refresh: 1800,
		Retry:   900,
		Expire:  604800,
		Minttl:  86400,
	}
}
