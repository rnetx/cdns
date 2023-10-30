//go:build linux

package internal

import (
	"net/netip"
	"time"

	"github.com/sagernet/netlink"
)

var _ IPSet = (*IPSetLinux)(nil)

type IPSetLinux struct {
	handler *netlink.Handle
}

func New() (IPSet, error) {
	handler, err := netlink.NewHandle()
	if err != nil {
		return nil, err
	}
	return &IPSetLinux{
		handler: handler,
	}, nil
}

func (i *IPSetLinux) Close() error {
	i.handler.Close()
	return nil
}

func (i *IPSetLinux) Create(name string, ttl time.Duration) error {
	if ttl < 0 {
		ttl = 0
	}
	timeout := uint32(ttl.Seconds())
	return i.handler.IpsetCreate(name, "hash:net", netlink.IpsetCreateOptions{
		Replace:  true,
		Skbinfo:  true,
		Revision: 1,
		Timeout:  &timeout,
	})
}

func (i *IPSetLinux) AddAddr(name string, addr netip.Addr, ttl time.Duration) error {
	if ttl < 0 {
		ttl = 0
	}
	timeout := uint32(ttl.Seconds())
	return i.handler.IpsetAdd(name, &netlink.IPSetEntry{
		Replace: true,
		IP:      addr.AsSlice(),
		Timeout: &timeout,
	})
}

func (i *IPSetLinux) AddPrefix(name string, prefix netip.Prefix, ttl time.Duration) error {
	if ttl < 0 {
		ttl = 0
	}
	timeout := uint32(ttl.Seconds())
	return i.handler.IpsetAdd(name, &netlink.IPSetEntry{
		Replace: true,
		IP:      prefix.Addr().AsSlice(),
		CIDR:    uint8(prefix.Bits()),
		Timeout: &timeout,
	})
}

func (i *IPSetLinux) DelAddr(name string, addr netip.Addr) error {
	return i.handler.IpsetDel(name, &netlink.IPSetEntry{
		IP: addr.AsSlice(),
	})
}

func (i *IPSetLinux) DelPrefix(name string, prefix netip.Prefix) error {
	return i.handler.IpsetDel(name, &netlink.IPSetEntry{
		IP:   prefix.Addr().AsSlice(),
		CIDR: uint8(prefix.Bits()),
	})
}

func (i *IPSetLinux) Flushall(name string) error {
	return i.handler.IpsetFlush(name)
}

func (i *IPSetLinux) Destroy(name string) error {
	return i.handler.IpsetDestroy(name)
}
