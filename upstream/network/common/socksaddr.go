package common

import (
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

type SocksAddr struct {
	domain string
	ip     netip.Addr
	port   uint16
}

func NewSocksIPPort(ip netip.Addr, port uint16) *SocksAddr {
	return &SocksAddr{
		ip:   ip,
		port: port,
	}
}

func NewSocksDomainPort(domain string, port uint16) *SocksAddr {
	return &SocksAddr{
		domain: domain,
		port:   port,
	}
}

func NewSocksAddrFromAddrPort(addr netip.AddrPort) *SocksAddr {
	return &SocksAddr{
		ip:   addr.Addr(),
		port: addr.Port(),
	}
}

func NewSocksAddrFromString(address string) (*SocksAddr, error) {
	addr, err := netip.ParseAddrPort(address)
	if err == nil {
		return NewSocksAddrFromAddrPort(addr), nil
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if host == "" {
		return nil, errors.New("invalid address")
	}
	portUint16, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, err
	}
	return &SocksAddr{
		domain: host,
		port:   uint16(portUint16),
	}, nil
}

func NewSocksAddrFromStringWithDefaultPort(address string, defaultPort uint16) (*SocksAddr, error) {
	addr, err := netip.ParseAddrPort(address)
	if err == nil {
		return NewSocksAddrFromAddrPort(addr), nil
	}
	ip, err := netip.ParseAddr(address)
	if err == nil {
		return NewSocksAddrFromAddrPort(netip.AddrPortFrom(ip, defaultPort)), nil
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		if strings.Contains(err.Error(), "missing port in address") {
			address = strings.Trim(address, "[]")
			ip, err := netip.ParseAddr(address)
			if err == nil {
				return &SocksAddr{
					ip:   ip,
					port: defaultPort,
				}, nil
			}
			return &SocksAddr{
				domain: address,
				port:   defaultPort,
			}, nil
		}
		return nil, err
	}
	if host == "" {
		return nil, errors.New("invalid address")
	}
	portUint16, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, err
	}
	return &SocksAddr{
		domain: host,
		port:   uint16(portUint16),
	}, nil
}

func (s *SocksAddr) String() string {
	if s.domain != "" {
		return net.JoinHostPort(s.domain, strconv.Itoa(int(s.port)))
	}
	return netip.AddrPortFrom(s.ip, s.port).String()
}

func (s *SocksAddr) Domain() string {
	return s.domain
}

func (s *SocksAddr) IP() netip.Addr {
	return s.ip
}

func (s *SocksAddr) Port() uint16 {
	return s.port
}

func (s *SocksAddr) IsDomain() bool {
	return s.domain != ""
}

func (s *SocksAddr) IsIP() bool {
	return s.ip.IsValid()
}

func (s *SocksAddr) IsIPv4() bool {
	return s.IsIP() && s.ip.Is4()
}

func (s *SocksAddr) IsIPv6() bool {
	return s.IsIP() && s.ip.Is6()
}

func (s *SocksAddr) UDPAddr() net.Addr {
	if s.IsIP() {
		return &net.UDPAddr{
			IP:   s.ip.AsSlice(),
			Port: int(s.port),
			Zone: s.ip.Zone(),
		}
	} else {
		return &DomainUDPAddr{
			domain: s.domain,
			port:   s.port,
		}
	}
}

type DomainUDPAddr struct {
	domain string
	port   uint16
}

func (d *DomainUDPAddr) Network() string {
	return "udp"
}

func (d *DomainUDPAddr) String() string {
	return net.JoinHostPort(d.domain, strconv.Itoa(int(d.port)))
}

var (
	fakeIPv4 = netip.MustParseAddr("255.127.0.1")
	fakeIPv6 = netip.MustParseAddr("fd00::1")
)

func FakeIPv4SocksAddr() *SocksAddr {
	return &SocksAddr{
		ip:   fakeIPv4,
		port: 1,
	}
}

func FakeIPv4SocksAddrWithPort(port uint16) *SocksAddr {
	return &SocksAddr{
		ip:   fakeIPv4,
		port: port,
	}
}

func FakeIPv6SocksAddr() *SocksAddr {
	return &SocksAddr{
		ip:   fakeIPv6,
		port: 1,
	}
}

func FakeIPv6SocksAddrWithPort(port uint16) *SocksAddr {
	return &SocksAddr{
		ip:   fakeIPv6,
		port: port,
	}
}
