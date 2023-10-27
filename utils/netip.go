package utils

import (
	"math/big"
	"math/rand"
	"net/netip"
	"time"
)

func RandomAddrFromPrefix(prefix netip.Prefix) netip.Addr {
	ip := prefix.Addr()
	startN := big.NewInt(0).SetBytes(ip.AsSlice())
	var bits int
	if ip.Is4() {
		bits = 5
	} else {
		bits = 7
	}
	bt := big.NewInt(0).Exp(big.NewInt(2), big.NewInt(1<<bits-int64(prefix.Bits())), nil)
	bt.Sub(bt, big.NewInt(2))
	n := big.NewInt(0).Rand(rand.New(rand.NewSource(time.Now().UnixNano())), bt)
	n.Add(n, startN)
	ip, _ = netip.AddrFromSlice(n.Bytes())
	return ip
}
