//go:build !darwin && !windows && (!linux || android)

package netinterface

import (
	"net"
	"os"
)

func GetDefaultInterfaceName() (*net.Interface, error) {
	return nil, os.ErrInvalid
}
