//go:build windows

package netinterface

import (
	"net"

	"github.com/rnetx/cdns/upstream/network/netinterface/internal/winipcfg"

	"golang.org/x/sys/windows"
)

func GetDefaultInterfaceName() (*net.Interface, error) {
	rows, err := winipcfg.GetIPForwardTable2(windows.AF_INET)
	if err != nil {
		return nil, err
	}

	lowestMetric := ^uint32(0)
	alias := ""

	for _, row := range rows {
		if row.DestinationPrefix.PrefixLength != 0 {
			continue
		}

		ifrow, err := row.InterfaceLUID.Interface()
		if err != nil || ifrow.OperStatus != winipcfg.IfOperStatusUp {
			continue
		}

		iface, err := row.InterfaceLUID.IPInterface(windows.AF_INET)
		if err != nil {
			continue
		}

		if ifrow.Type == winipcfg.IfTypePropVirtual || ifrow.Type == winipcfg.IfTypeSoftwareLoopback {
			continue
		}

		metric := row.Metric + iface.Metric
		if metric < lowestMetric {
			lowestMetric = metric
			alias = ifrow.Alias()
		}
	}

	if alias == "" {
		return nil, ErrNoRoute
	}

	return net.InterfaceByName(alias)
}
