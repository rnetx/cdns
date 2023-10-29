package internal

import (
	"net/netip"
	"time"
)

type IPSet interface {
	Create(name string, ttl time.Duration) error
	Close() error
	AddAddr(name string, addr netip.Addr, ttl time.Duration) error
	AddPrefix(name string, prefix netip.Prefix, ttl time.Duration) error
	DelAddr(name string, addr netip.Addr) error
	DelPrefix(name string, prefix netip.Prefix) error
	Flushall(name string) error
	Destroy(name string) error
}
