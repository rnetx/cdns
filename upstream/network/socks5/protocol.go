package socks5

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/netip"

	"github.com/rnetx/cdns/upstream/network/common"
)

const (
	Version byte = 5

	AuthTypeNotRequired       byte = 0x00
	AuthTypeGSSAPI            byte = 0x01
	AuthTypeUsernamePassword  byte = 0x02
	AuthTypeNoAcceptedMethods byte = 0xFF

	UsernamePasswordStatusSuccess byte = 0x00
	UsernamePasswordStatusFailure byte = 0x01

	CommandConnect      byte = 0x01
	CommandBind         byte = 0x02
	CommandUDPAssociate byte = 0x03

	ReplyCodeSuccess                byte = 0
	ReplyCodeFailure                byte = 1
	ReplyCodeNotAllowed             byte = 2
	ReplyCodeNetworkUnreachable     byte = 3
	ReplyCodeHostUnreachable        byte = 4
	ReplyCodeConnectionRefused      byte = 5
	ReplyCodeTTLExpired             byte = 6
	ReplyCodeUnsupported            byte = 7
	ReplyCodeAddressTypeUnsupported byte = 8
)

func writeErrHandle(err error) error {
	return fmt.Errorf("write failed: %s", err)
}

func readErrHandle(err error) error {
	return fmt.Errorf("read failed: %s", err)
}

func must(errs ...error) {
	for _, err := range errs {
		if err != nil {
			panic(err)
		}
	}
}

func must1[T any](result T, err error) {
	if err != nil {
		panic(err)
	}
}

// +----+----------+----------+
// |VER | NMETHODS | METHODS  |
// +----+----------+----------+
// | 1  |    1     | 1 to 255 |
// +----+----------+----------+

type AuthRequest struct {
	Methods []byte
}

func (r *AuthRequest) WriteRequest(writer io.Writer) error {
	buffer := bytes.NewBuffer(make([]byte, 0, 2+len(r.Methods)))
	must(
		buffer.WriteByte(Version),
		buffer.WriteByte(byte(len(r.Methods))),
	)
	must1(buffer.Write(r.Methods))
	_, err := buffer.WriteTo(writer)
	return err
}

// +----+--------+
// |VER | METHOD |
// +----+--------+
// | 1  |   1    |
// +----+--------+

type AuthResponse struct {
	Method byte
}

func (r *AuthResponse) ReadResponse(reader io.Reader) error {
	var buffer [2]byte
	_, err := io.ReadFull(reader, buffer[:])
	if err != nil {
		return readErrHandle(err)
	}
	if buffer[0] != Version {
		return fmt.Errorf("invalid version: %d", buffer[0])
	}
	if buffer[1] == AuthTypeNoAcceptedMethods {
		return fmt.Errorf("no accepted methods")
	}
	r.Method = buffer[1]
	return nil
}

// +----+------+----------+------+----------+
// |VER | ULEN |  UNAME   | PLEN |  PASSWD  |
// +----+------+----------+------+----------+
// | 1  |  1   | 1 to 255 |  1   | 1 to 255 |
// +----+------+----------+------+----------+

type UsernamePasswordAuthRequest struct {
	Username string
	Password string
}

func (r *UsernamePasswordAuthRequest) WriteRequest(writer io.Writer) error {
	buffer := bytes.NewBuffer(make([]byte, 0, 3+len(r.Username)+len(r.Password)))
	must(
		buffer.WriteByte(Version),
		buffer.WriteByte(byte(len(r.Username))),
	)
	must1(buffer.WriteString(r.Username))
	must(buffer.WriteByte(byte(len(r.Password))))
	must1(buffer.WriteString(r.Password))
	_, err := buffer.WriteTo(writer)
	return err
}

// +----+--------+
// |VER | STATUS |
// +----+--------+
// | 1  |   1    |
// +----+--------+

type UsernamePasswordAuthResponse struct {
	Status byte
}

func (r *UsernamePasswordAuthResponse) ReadResponse(reader io.Reader) error {
	buffer := make([]byte, 2)
	_, err := io.ReadFull(reader, buffer)
	if err != nil {
		return readErrHandle(err)
	}
	if buffer[0] != Version {
		return fmt.Errorf("invalid version: %d", buffer[0])
	}
	r.Status = buffer[1]
	return nil
}

// +----+-----+-------+------+----------+----------+
// |VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
// +----+-----+-------+------+----------+----------+
// | 1  |  1  | X'00' |  1   | Variable |    2     |
// +----+-----+-------+------+----------+----------+

type CommandRequest struct {
	Command     byte
	Destination common.SocksAddr
}

func (r *CommandRequest) WriteRequest(writer io.Writer) error {
	buffer := bytes.NewBuffer(make([]byte, 0, 3+socksAddrLen(&r.Destination)))
	must(
		buffer.WriteByte(Version),
		buffer.WriteByte(r.Command),
		buffer.WriteByte(0x00),
		writeSocksAddr(buffer, &r.Destination),
	)
	_, err := buffer.WriteTo(writer)
	return err
}

// +----+-----+-------+------+----------+----------+
// |VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
// +----+-----+-------+------+----------+----------+
// | 1  |  1  | X'00' |  1   | Variable |    2     |
// +----+-----+-------+------+----------+----------+

type CommandResponse struct {
	ReplyCode byte
	Bind      common.SocksAddr
}

func (r *CommandResponse) ReadResponse(reader io.Reader) error {
	buffer := make([]byte, 261)
	var err error
	_, err = reader.Read(buffer)
	if err != nil {
		return readErrHandle(err)
	}
	if buffer[0] != Version {
		return fmt.Errorf("invalid version: %d", buffer[0])
	}
	r.ReplyCode = buffer[1]
	bind, err := readSocksAddr(bytes.NewReader(buffer[3:]))
	if err != nil {
		return err
	}
	r.Bind = *bind
	return nil
}

//

func socksAddrLen(addr *common.SocksAddr) int {
	if addr.IsDomain() {
		return 1 + 1 + len(addr.Domain()) + 2
	}
	if addr.IsIPv4() {
		return 1 + 4 + 2
	} else {
		return 1 + 16 + 2
	}
}

func writeSocksAddr(writer io.Writer, addr *common.SocksAddr) error {
	buffer := bytes.NewBuffer(make([]byte, 0, socksAddrLen(addr)))
	var err error
	switch {
	case addr.IsDomain():
		must(
			buffer.WriteByte(0x03), // Domain
			buffer.WriteByte(byte(len(addr.Domain()))),
		)
		must1(buffer.WriteString(addr.Domain()))
	case addr.IsIPv4():
		must(buffer.WriteByte(0x01)) // IPv4
		must1(buffer.Write(addr.IP().AsSlice()))
	case addr.IsIPv6():
		must(buffer.WriteByte(0x04)) // IPv6
		must1(buffer.Write(addr.IP().AsSlice()))
	}
	must(binary.Write(buffer, binary.BigEndian, addr.Port()))
	_, err = buffer.WriteTo(writer)
	return err
}

func readSocksAddr(reader io.Reader) (*common.SocksAddr, error) {
	aType := make([]byte, 1)
	_, err := io.ReadFull(reader, aType)
	if err != nil {
		return nil, readErrHandle(err)
	}
	var (
		domain string
		ip     *netip.Addr
	)
	switch aType[0] {
	case 0x03: // Domain
		domainLen := make([]byte, 1)
		_, err = io.ReadFull(reader, domainLen)
		if err != nil {
			return nil, readErrHandle(err)
		}
		domainBytes := make([]byte, domainLen[0])
		_, err = io.ReadFull(reader, domainBytes)
		if err != nil {
			return nil, readErrHandle(err)
		}
		domain = string(domainBytes)
	case 0x01: // IPv4
		ipBytes := make([]byte, 4)
		_, err = io.ReadFull(reader, ipBytes)
		if err != nil {
			return nil, readErrHandle(err)
		}
		ip = new(netip.Addr)
		*ip = netip.AddrFrom4([4]byte(ipBytes))
	case 0x04: // IPv6
		ipBytes := make([]byte, 16)
		_, err = io.ReadFull(reader, ipBytes)
		if err != nil {
			return nil, readErrHandle(err)
		}
		ip = new(netip.Addr)
		*ip = netip.AddrFrom16([16]byte(ipBytes))
	default:
		return nil, fmt.Errorf("invalid address type: %d", aType[0])
	}
	var port uint16
	err = binary.Read(reader, binary.BigEndian, &port)
	if err != nil {
		return nil, readErrHandle(err)
	}
	if ip != nil {
		return common.NewSocksIPPort(*ip, port), nil
	} else {
		return common.NewSocksDomainPort(domain, port), nil
	}
}
