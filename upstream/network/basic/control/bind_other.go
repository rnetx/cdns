//go:build !(linux || windows || darwin)

package control

import "syscall"

func bindToInterface(conn syscall.RawConn, _ bool, interfaceName string, interfaceIndex int) error {
	return nil
}
