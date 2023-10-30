//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package control

import "syscall"

func ReuseAddr() func(network, address string, conn syscall.RawConn) error {
	return nil
}
