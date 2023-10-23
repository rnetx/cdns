package network

import (
	"context"
	"net"
	"net/netip"

	"github.com/rnetx/cdns/upstream/network/basic"
	"github.com/rnetx/cdns/upstream/network/common"
	"github.com/rnetx/cdns/upstream/network/socks5"
	"github.com/rnetx/cdns/utils"
)

type Options struct {
	BasicOptions  basic.Options   `yaml:",inline"`
	Socks5Options *socks5.Options `yaml:"socks5,omitempty"`
}

func NewDialer(options Options) (common.Dialer, error) {
	dialer, err := basic.NewDialer(options.BasicOptions)
	if err != nil {
		return nil, err
	}
	if options.Socks5Options != nil {
		dialer, err = socks5.NewDialer(dialer, *options.Socks5Options)
		if err != nil {
			return nil, err
		}
	}
	return dialer, nil
}

func IsSocks5Dialer(dialer common.Dialer) bool {
	_, ok := dialer.(*socks5.Dialer)
	return ok
}

type dialParallelResult struct {
	conn net.Conn
	ip   netip.Addr
}

func DialParallel(ctx context.Context, dialer common.Dialer, network string, ips []netip.Addr, port uint16) (net.Conn, netip.Addr, error) {
	safeChan := utils.NewSafeChan[utils.Result[dialParallelResult]](len(ips))
	defer safeChan.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, ip := range ips {
		go func(safeChan *utils.SafeChan[utils.Result[dialParallelResult]], ip netip.Addr) {
			defer safeChan.Close()
			conn, err := dialer.DialContext(ctx, network, *common.NewSocksIPPort(ip, port))
			if err != nil {
				select {
				case safeChan.SendChan() <- utils.Result[dialParallelResult]{Error: err}:
				case <-ctx.Done():
				}
			} else {
				select {
				case safeChan.SendChan() <- utils.Result[dialParallelResult]{Value: dialParallelResult{conn: conn, ip: ip}}:
				case <-ctx.Done():
				}
			}
		}(safeChan.Clone(), ip)
	}
	var lastErr error
	for i := 0; i < len(ips); i++ {
		select {
		case <-ctx.Done():
			return nil, netip.Addr{}, ctx.Err()
		case result := <-safeChan.ReceiveChan():
			if result.Error != nil {
				lastErr = result.Error
				continue
			}
			dialResult := result.Value
			return dialResult.conn, dialResult.ip, nil
		}
	}
	return nil, netip.Addr{}, lastErr
}

func ListenParallel(ctx context.Context, dialer common.Dialer, ips []netip.Addr, port uint16) (net.PacketConn, netip.Addr, error) {
	// TODO: Use First IP
	ip := ips[0]
	socksAddr := common.NewSocksIPPort(ip, port)
	conn, err := dialer.ListenPacket(ctx, *socksAddr)
	if err != nil {
		return nil, netip.Addr{}, err
	}
	return conn, ip, nil
}
