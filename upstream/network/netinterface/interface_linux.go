//go:build linux && !android

package netinterface

import (
	"net"

	"github.com/sagernet/netlink"
	"golang.org/x/sys/unix"
)

func GetDefaultInterfaceName() (*net.Interface, error) {
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_ALL, &netlink.Route{Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return nil, err
	}

	for _, route := range routes {
		if route.Dst != nil {
			continue
		}

		var link netlink.Link
		link, err = netlink.LinkByIndex(route.LinkIndex)
		if err != nil {
			return nil, err
		}

		return net.InterfaceByName(link.Attrs().Name)
	}

	return nil, ErrNoRoute
}
