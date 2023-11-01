package control

import (
	"net"
	"syscall"
)

func BindToInterface(interfaceName string, isIPv6 bool) func(network string, address string, c syscall.RawConn) error {
	return func(network string, address string, rawConn syscall.RawConn) error {
		iface, err := net.InterfaceByName(interfaceName)
		if err != nil {
			return err
		}
		return bindToInterface(rawConn, isIPv6, interfaceName, iface.Index)
	}
}
