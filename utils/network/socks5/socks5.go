package socks5

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	"github.com/rnetx/cdns/utils/network/common"
)

type Options struct {
	Address  netip.AddrPort `yaml:"address,omitempty"`
	Username string         `yaml:"username,omitempty"`
	Password string         `yaml:"password,omitempty"`
}

type Dialer struct {
	dialer   common.Dialer
	address  common.SocksAddr
	username string
	password string
}

func NewDialer(dialer common.Dialer, options Options) (common.Dialer, error) {
	if (options.Username == "" && options.Password != "") || (options.Username != "" && options.Password == "") {
		return nil, fmt.Errorf("invalid username and password")
	}
	return &Dialer{
		dialer:   dialer,
		address:  *common.NewSocksAddrFromAddrPort(options.Address),
		username: options.Username,
		password: options.Password,
	}, nil
}

func (d *Dialer) newTCPBasicConn(ctx context.Context) (conn net.Conn, err error) {
	conn, err = d.dialer.DialContext(ctx, "tcp", d.address)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()
	// Auth Request
	authRequest := &AuthRequest{}
	if d.username != "" && d.password != "" {
		authRequest.Methods = []byte{AuthTypeNotRequired, AuthTypeUsernamePassword}
	} else {
		authRequest.Methods = []byte{AuthTypeNotRequired}
	}
	err = authRequest.WriteRequest(conn)
	if err != nil {
		return
	}
	var authResponse AuthResponse
	err = authResponse.ReadResponse(conn)
	if err != nil {
		return
	}
	if authResponse.Method == AuthTypeUsernamePassword && (d.username != "" && d.password != "") {
		// Username/Password Auth
		usernamePasswordAuthRequest := &UsernamePasswordAuthRequest{
			Username: d.username,
			Password: d.password,
		}
		err = usernamePasswordAuthRequest.WriteRequest(conn)
		if err != nil {
			return
		}
		var usernamePasswordAuthResponse UsernamePasswordAuthResponse
		err = usernamePasswordAuthResponse.ReadResponse(conn)
		if err != nil {
			return
		}
		if usernamePasswordAuthResponse.Status != UsernamePasswordStatusSuccess {
			err = fmt.Errorf("auth failed: invalid username or password")
			return
		}
	}
	return
}

func (d *Dialer) DialContext(ctx context.Context, network string, address common.SocksAddr) (conn net.Conn, err error) {
	isTCP := false
	switch network {
	case "tcp4", "tcp6", "tcp":
		isTCP = true
	case "udp4", "udp6", "udp":
	default:
		err = fmt.Errorf("invalid network: %s", network)
		return
	}
	conn, err = d.newTCPBasicConn(ctx)
	if err != nil {
		return
	}
	commandRequest := &CommandRequest{
		Destination: address,
	}
	if isTCP {
		// TCP
		commandRequest.Command = CommandConnect
	} else {
		// UDP(Connected)
		commandRequest.Command = CommandUDPAssociate
	}
	err = commandRequest.WriteRequest(conn)
	if err != nil {
		conn.Close()
		return
	}
	var commandResponse CommandResponse
	err = commandResponse.ReadResponse(conn)
	if err != nil {
		conn.Close()
		return
	}
	err = replyCodeErr(commandResponse.ReplyCode)
	if err != nil {
		conn.Close()
		return
	}
	if isTCP {
		return
	}
	if commandResponse.Bind.IsDomain() {
		return nil, fmt.Errorf("bind: domain address is not supported: %s", commandResponse.Bind.String())
	}
	udpConn, err := d.dialer.DialContext(ctx, "udp", commandResponse.Bind)
	if err != nil {
		conn.Close()
		return
	}
	return &AssociatePacketConn{
		Conn:              udpConn,
		tcpConn:           conn,
		udpRealRemoteAddr: address,
	}, nil
}

func (d *Dialer) ListenPacket(ctx context.Context, address common.SocksAddr) (net.PacketConn, error) {
	udpConn, err := d.DialContext(ctx, "udp", address)
	if err != nil {
		return nil, err
	}
	return udpConn.(*AssociatePacketConn), nil
}

func replyCodeErr(replyCode byte) (err error) {
	switch replyCode {
	case ReplyCodeSuccess:
	case ReplyCodeFailure:
		err = fmt.Errorf("command failed: reply: failure")
	case ReplyCodeNotAllowed:
		err = fmt.Errorf("command failed: reply: not allowed")
	case ReplyCodeNetworkUnreachable:
		err = fmt.Errorf("command failed: reply: network unreachable")
	case ReplyCodeHostUnreachable:
		err = fmt.Errorf("command failed: reply: host unreachable")
	case ReplyCodeConnectionRefused:
		err = fmt.Errorf("command failed: reply: connection refused")
	case ReplyCodeTTLExpired:
		err = fmt.Errorf("command failed: reply: ttl expired")
	case ReplyCodeUnsupported:
		err = fmt.Errorf("command failed: reply: unsupported")
	case ReplyCodeAddressTypeUnsupported:
		err = fmt.Errorf("command failed: reply: address type unsupported")
	default:
		err = fmt.Errorf("command failed: reply: unknown reply code: %d", replyCode)
	}
	return
}
