package basic

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"syscall"

	"github.com/rnetx/cdns/upstream/network/basic/control"
	"github.com/rnetx/cdns/upstream/network/common"
)

type Options struct {
	BindInterface string      `yaml:"bind-interface,omitempty"`
	BindIPv4      *netip.Addr `yaml:"bind-ipv4,omitempty"`
	BindIPv6      *netip.Addr `yaml:"bind-ipv6,omitempty"`
	SoMark        *uint32     `yaml:"so-mark,omitempty"`
}

type Dialer struct {
	tcp4Dialer   *net.Dialer
	tcp6Dialer   *net.Dialer
	udp4Dialer   *net.Dialer
	udp6Dialer   *net.Dialer
	udp4Addr     string
	udp6Addr     string
	udp4Listener net.ListenConfig
	udp6Listener net.ListenConfig
}

func NewDialer(options Options) (common.Dialer, error) {
	var (
		dialer4   net.Dialer
		dialer6   net.Dialer
		listener4 net.ListenConfig
		listener6 net.ListenConfig
		c4        []func(network string, address string, c syscall.RawConn) error
		c6        []func(network string, address string, c syscall.RawConn) error
	)
	if options.BindInterface != "" {
		c4 = append(c4, control.BindToInterface(options.BindInterface, false))
		c6 = append(c6, control.BindToInterface(options.BindInterface, true))
	}
	if options.SoMark != nil {
		c4 = append(c4, control.SetMark(*options.SoMark))
		c6 = append(c6, control.SetMark(*options.SoMark))
	}
	if c4 != nil {
		dialer4.Control = control.AppendControl(c4...)
		listener4.Control = dialer4.Control
	}
	if c6 != nil {
		dialer6.Control = control.AppendControl(c6...)
		listener6.Control = dialer6.Control
	}
	var (
		tcpDialer4 = dialer4
		udpDialer4 = dialer4
		udpAddr4   = ""
	)
	if options.BindIPv4 != nil {
		if !options.BindIPv4.Is4() {
			return nil, fmt.Errorf("invalid bind-ipv4: %s", options.BindIPv4.String())
		}
		tcpDialer4.LocalAddr = &net.TCPAddr{IP: options.BindIPv4.AsSlice()}
		udpDialer4.LocalAddr = &net.UDPAddr{IP: options.BindIPv4.AsSlice()}
		udpAddr4 = netip.AddrPortFrom(*options.BindIPv4, 0).String()
	}
	if udpAddr4 == "" {
		udpAddr4 = netip.AddrPortFrom(netip.IPv4Unspecified(), 0).String()
	}
	var (
		tcpDialer6 = dialer6
		udpDialer6 = dialer6
		udpAddr6   = ""
	)
	if options.BindIPv6 != nil {
		if !options.BindIPv6.Is6() {
			return nil, fmt.Errorf("invalid bind-ipv6: %s", options.BindIPv6.String())
		}
		tcpDialer6.LocalAddr = &net.TCPAddr{IP: options.BindIPv6.AsSlice()}
		udpDialer6.LocalAddr = &net.UDPAddr{IP: options.BindIPv6.AsSlice()}
		udpAddr6 = netip.AddrPortFrom(*options.BindIPv6, 0).String()
	}
	if udpAddr6 == "" {
		udpAddr6 = netip.AddrPortFrom(netip.IPv6Unspecified(), 0).String()
	}
	d := &Dialer{
		tcp4Dialer:   &tcpDialer4,
		tcp6Dialer:   &tcpDialer6,
		udp4Dialer:   &udpDialer4,
		udp6Dialer:   &udpDialer6,
		udp4Addr:     udpAddr4,
		udp6Addr:     udpAddr6,
		udp4Listener: listener4,
		udp6Listener: listener6,
	}
	return d, nil
}

func (d *Dialer) DialContext(ctx context.Context, network string, address common.SocksAddr) (net.Conn, error) {
	if address.IsDomain() {
		return nil, fmt.Errorf("domain address is not supported: %s", address.String())
	}
	switch network {
	case "tcp4":
		return d.tcp4Dialer.DialContext(ctx, network, address.String())
	case "tcp6":
		return d.tcp6Dialer.DialContext(ctx, network, address.String())
	case "tcp":
		if address.IsIPv4() {
			return d.tcp4Dialer.DialContext(ctx, "tcp4", address.String())
		} else {
			return d.tcp6Dialer.DialContext(ctx, "tcp6", address.String())
		}
	case "udp4":
		return d.udp4Dialer.DialContext(ctx, network, address.String())
	case "udp6":
		return d.udp6Dialer.DialContext(ctx, network, address.String())
	case "udp":
		if address.IsIPv4() {
			return d.udp4Dialer.DialContext(ctx, "udp4", address.String())
		} else {
			return d.udp6Dialer.DialContext(ctx, "udp6", address.String())
		}
	default:
		return nil, fmt.Errorf("invalid network: %s", network)
	}
}

func (d *Dialer) ListenPacket(ctx context.Context, address common.SocksAddr) (net.PacketConn, error) {
	if address.IsDomain() {
		return nil, fmt.Errorf("domain address is not supported: %s", address.String())
	}
	if address.IsIPv4() {
		return d.udp4Listener.ListenPacket(ctx, "udp4", d.udp4Addr)
	} else {
		return d.udp6Listener.ListenPacket(ctx, "udp6", d.udp6Addr)
	}
}
