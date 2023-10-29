//go:build !linux

package internal

import (
	"errors"
	"net/netip"
	"time"
)

var ErrOsUnsupported = errors.New("os unsupported")

var _ IPSet = (*IPSetOther)(nil)

type IPSetOther struct{}

func New() (*IPSetOther, error) {
	return &IPSetOther{}, nil
}

func (i *IPSetOther) Close() error {
	return ErrOsUnsupported
}

func (i *IPSetOther) Create(_ string, _ time.Duration) error {
	return ErrOsUnsupported
}

func (i *IPSetOther) AddAddr(_ string, _ netip.Addr, _ time.Duration) error {
	return ErrOsUnsupported
}

func (i *IPSetOther) AddPrefix(_ string, _ netip.Prefix, _ time.Duration) error {
	return ErrOsUnsupported
}

func (i *IPSetOther) DelAddr(_ string, _ netip.Addr) error {
	return ErrOsUnsupported
}

func (i *IPSetOther) DelPrefix(_ string, _ netip.Prefix) error {
	return ErrOsUnsupported
}

func (i *IPSetOther) Flushall(_ string) error {
	return ErrOsUnsupported
}

func (i *IPSetOther) Destroy(_ string) error {
	return ErrOsUnsupported
}
