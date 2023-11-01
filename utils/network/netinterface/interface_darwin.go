//go:build darwin

package netinterface

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"time"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

func GetDefaultInterfaceName() (*net.Interface, error) {
	ribMessage, err := route.FetchRIB(unix.AF_UNSPEC, route.RIBTypeRoute, 0)
	if err != nil {
		return nil, err
	}
	routeMessages, err := route.ParseRIB(route.RIBTypeRoute, ribMessage)
	if err != nil {
		return nil, err
	}
	var defaultInterface *net.Interface
	for _, rawRouteMessage := range routeMessages {
		routeMessage := rawRouteMessage.(*route.RouteMessage)
		if len(routeMessage.Addrs) <= unix.RTAX_NETMASK {
			continue
		}
		destination, isIPv4Destination := routeMessage.Addrs[unix.RTAX_DST].(*route.Inet4Addr)
		if !isIPv4Destination {
			continue
		}
		if destination.IP != netip.IPv4Unspecified().As4() {
			continue
		}
		mask, isIPv4Mask := routeMessage.Addrs[unix.RTAX_NETMASK].(*route.Inet4Addr)
		if !isIPv4Mask {
			continue
		}
		ones, _ := net.IPMask(mask.IP[:]).Size()
		if ones != 0 {
			continue
		}
		routeInterface, err := net.InterfaceByIndex(routeMessage.Index)
		if err != nil {
			return nil, err
		}
		if routeMessage.Flags&unix.RTF_UP == 0 {
			continue
		}
		if routeMessage.Flags&unix.RTF_GATEWAY == 0 {
			continue
		}
		// if routeMessage.Flags&unix.RTF_IFSCOPE != 0 {
		// continue
		// }
		defaultInterface = routeInterface
		break
	}
	if defaultInterface == nil {
		defaultInterface, err = getDefaultInterfaceBySocket()
		if err != nil {
			return nil, err
		}
	}
	if defaultInterface == nil {
		return nil, ErrNoRoute
	}
	return defaultInterface, nil
}

func getDefaultInterfaceBySocket() (*net.Interface, error) {
	socketFd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, fmt.Errorf("create file descriptor: %w", err)
	}
	defer unix.Close(socketFd)
	go unix.Connect(socketFd, &unix.SockaddrInet4{
		Addr: [4]byte{10, 255, 255, 255},
		Port: 80,
	})
	result := make(chan netip.Addr, 1)
	go func() {
		for {
			sockname, sockErr := unix.Getsockname(socketFd)
			if sockErr != nil {
				break
			}
			sockaddr, isInet4Sockaddr := sockname.(*unix.SockaddrInet4)
			if !isInet4Sockaddr {
				break
			}
			addr := netip.AddrFrom4(sockaddr.Addr)
			if addr.IsUnspecified() {
				time.Sleep(time.Millisecond)
				continue
			}
			result <- addr
			break
		}
	}()
	var selectedAddr netip.Addr
	select {
	case selectedAddr = <-result:
	case <-time.After(time.Second):
		return nil, os.ErrDeadlineExceeded
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, netInterface := range interfaces {
		interfaceAddrs, err := netInterface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, interfaceAddr := range interfaceAddrs {
			ipNet, isIPNet := interfaceAddr.(*net.IPNet)
			if !isIPNet {
				continue
			}
			if ipNet.Contains(selectedAddr.AsSlice()) {
				return &netInterface, nil
			}
		}
	}
	return nil, fmt.Errorf("no interface found for address %s", selectedAddr)
}
