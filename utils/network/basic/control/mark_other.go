//go:build !linux

package control

import "syscall"

func SetMark(mark uint32) func(network string, address string, c syscall.RawConn) error {
	return nil
}
