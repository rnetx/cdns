package socks5

import (
	"bytes"
	"fmt"
	"net"
	"syscall"

	"github.com/rnetx/cdns/utils/network/common"
)

// +----+------+------+----------+----------+----------+
// |RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
// +----+------+------+----------+----------+----------+
// | 2  |  1   |  1   | Variable |    2     | Variable |
// +----+------+------+----------+----------+----------+

var (
	_ net.Conn       = (*AssociatePacketConn)(nil)
	_ net.PacketConn = (*AssociatePacketConn)(nil)
)

type AssociatePacketConn struct {
	net.Conn
	tcpConn           net.Conn
	udpRealRemoteAddr common.SocksAddr
}

func (c *AssociatePacketConn) RemoteAddr() net.Addr {
	return c.udpRealRemoteAddr.UDPAddr()
}

func (c *AssociatePacketConn) Close() error {
	defer c.tcpConn.Close()
	return c.Conn.Close()
}

func (c *AssociatePacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = c.Conn.Read(p)
	if err != nil {
		return
	}
	reader := bytes.NewReader(p[:n])
	if reader.Len() < 4 {
		n = 0
		err = fmt.Errorf("invalid udp packet")
		return
	}
	header := make([]byte, 3)
	_, err = reader.Read(header)
	if err != nil {
		n = 0
		return
	}
	if header[0] != 0x00 || header[1] != 0x00 || header[2] != 0x00 {
		n = 0
		err = fmt.Errorf("invalid udp packet")
		return
	}
	socksAddr, err := readSocksAddr(reader)
	if err != nil {
		n = 0
		err = fmt.Errorf("invalid udp packet")
		return
	}
	index := 3 + socksAddrLen(socksAddr)
	n = copy(p, p[index:n])
	addr = socksAddr.UDPAddr()
	return
}

func (c *AssociatePacketConn) Read(p []byte) (n int, err error) {
	n, _, err = c.ReadFrom(p)
	return
}

func (c *AssociatePacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	socksAddr, err := common.NewSocksAddrFromString(addr.String())
	if err != nil {
		err = fmt.Errorf("invalid udp address: %s", err)
		return
	}
	buffer := bytes.NewBuffer(make([]byte, 0, len(p)+3+socksAddrLen(socksAddr)))
	buffer.Write([]byte{0x00, 0x00, 0x00})
	err = writeSocksAddr(buffer, socksAddr)
	if err != nil {
		return
	}
	buffer.Write(p)
	_, err = c.Conn.Write(buffer.Bytes())
	if err != nil {
		return
	}
	n = len(p)
	return
}

func (c *AssociatePacketConn) Write(p []byte) (n int, err error) {
	buffer := bytes.NewBuffer(make([]byte, 0, len(p)+3+socksAddrLen(&c.udpRealRemoteAddr)))
	buffer.Write([]byte{0x00, 0x00, 0x00})
	err = writeSocksAddr(buffer, &c.udpRealRemoteAddr)
	if err != nil {
		return
	}
	buffer.Write(p)
	_, err = c.Conn.Write(buffer.Bytes())
	if err != nil {
		return
	}
	n = len(p)
	return
}

// QUIC
func (c *AssociatePacketConn) SetReadBuffer(n int) error {
	udpConn := c.Conn.(*net.UDPConn)
	return udpConn.SetReadBuffer(n + 261)
}

// QUIC
func (c *AssociatePacketConn) SetWriteBuffer(n int) error {
	udpConn := c.Conn.(*net.UDPConn)
	return udpConn.SetWriteBuffer(n + 261)
}

// QUIC
func (c *AssociatePacketConn) SyscallConn() (syscall.RawConn, error) {
	conn, ok := c.Conn.(interface {
		SyscallConn() (syscall.RawConn, error)
	})
	if !ok {
		return nil, fmt.Errorf("not support syscall conn")
	}
	return conn.SyscallConn()
}
