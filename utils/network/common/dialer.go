package common

import (
	"context"
	"net"
)

type Dialer interface {
	DialContext(ctx context.Context, network string, address SocksAddr) (net.Conn, error)
	ListenPacket(ctx context.Context, address SocksAddr) (net.PacketConn, error)
}
