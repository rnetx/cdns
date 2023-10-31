package upstream

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/upstream/network/basic/control"
	"github.com/rnetx/cdns/upstream/network/netinterface"
	"github.com/rnetx/cdns/utils"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/miekg/dns"
)

type DHCPUpstreamOptions struct {
	Interface          string         `yaml:"interface"`
	UseIPv6            bool           `yaml:"use-ipv6,omitempty"`
	ConnectTimeout     utils.Duration `yaml:"connect-timeout,omitempty"`
	IdleTimeout        utils.Duration `yaml:"idle-timeout,omitempty"`
	EDNS0              bool           `yaml:"edns0,omitempty"`
	DisableFallbackTCP bool           `yaml:"disable-fallback-tcp,omitempty"`
	EnablePipeline     bool           `yaml:"enable-pipeline,omitempty"`
}

const (
	DHCPUpstreamType          = "dhcp"
	DHCPDefaultRequestTimeout = 5 * time.Second
)

var (
	_ adapter.Upstream = (*DHCPUpstream)(nil)
	_ adapter.Starter  = (*DHCPUpstream)(nil)
	_ adapter.Closer   = (*DHCPUpstream)(nil)
)

type DHCPUpstream struct {
	ctx    context.Context
	core   adapter.Core
	tag    string
	logger log.Logger

	listenerConfig net.ListenConfig

	interfaceName string
	useIPv6       bool

	connectTimeout time.Duration
	idleTimeout    time.Duration

	edns0 bool

	disableFallbackTCP bool
	enablePipeline     bool

	oldInterfaceName  string
	oldInterfaceIndex int
	fetchCtx          context.Context
	fetchCancel       context.CancelFunc
	fetchTaskGroup    *utils.TaskGroup
	upstreamAddresses []string
	upstreamMap       map[string]adapter.Upstream

	reqTotal   atomic.Uint64
	reqSuccess atomic.Uint64
}

func NewDHCPUpstream(ctx context.Context, core adapter.Core, logger log.Logger, tag string, options DHCPUpstreamOptions) (adapter.Upstream, error) {
	u := &DHCPUpstream{
		ctx:    ctx,
		core:   core,
		tag:    tag,
		logger: logger,
	}
	u.interfaceName = options.Interface
	u.useIPv6 = false // TODO: IPv6
	if options.ConnectTimeout > 0 {
		u.connectTimeout = time.Duration(options.ConnectTimeout)
	} else {
		u.connectTimeout = DefaultConnectTimeout
	}
	if options.IdleTimeout > 0 {
		u.idleTimeout = time.Duration(options.IdleTimeout)
	} else {
		u.idleTimeout = DefaultIdleTimeout
	}
	u.disableFallbackTCP = options.DisableFallbackTCP
	u.enablePipeline = options.EnablePipeline
	u.edns0 = options.EDNS0
	return u, nil
}

func (u *DHCPUpstream) Tag() string {
	return u.tag
}

func (u *DHCPUpstream) Type() string {
	return DHCPUpstreamType
}

func (u *DHCPUpstream) Dependencies() []string {
	return nil
}

func (u *DHCPUpstream) Start() error {
	err := u.fetchDNSUpstream(u.ctx)
	if err != nil {
		return fmt.Errorf("start dhcp upstream failed: %s", err)
	}
	u.fetchCtx, u.fetchCancel = context.WithCancel(u.ctx)
	u.fetchTaskGroup = utils.NewTaskGroup()
	go u.loopFetch()
	return nil
}

func (u *DHCPUpstream) Close() error {
	u.fetchCancel()
	<-u.fetchTaskGroup.Wait()
	for _, uu := range u.upstreamMap {
		closer, isCloser := uu.(adapter.Closer)
		if isCloser {
			err := closer.Close()
			if err != nil {
				u.logger.Errorf("close upstream[%s] failed: %s", uu.Tag(), err)
			} else {
				u.logger.Infof("close upstream[%s] success", uu.Tag())
			}
		}
	}
	return nil
}

func (u *DHCPUpstream) loopFetch() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-u.fetchCtx.Done():
			return
		case <-ticker.C:
			t := u.fetchTaskGroup.AddTask()
			if utils.IsContextCancelled(u.fetchCtx) {
				t.Done()
				return
			}
			err := u.fetchDNSUpstream(u.fetchCtx)
			if err != nil {
				u.logger.Errorf("fetch dhcp upstream failed: %s", err)
			}
			t.Done()
		}
	}
}

func (u *DHCPUpstream) fetchDNSUpstream(ctx context.Context) error {
	var (
		iface *net.Interface
		err   error
	)
	if u.interfaceName == "" {
		iface, err = netinterface.GetDefaultInterfaceName()
		if err != nil {
			return err
		}
	} else {
		iface, err = net.InterfaceByName(u.interfaceName)
	}
	if err != nil {
		return err
	}
	if iface.Name == u.oldInterfaceName && iface.Index == u.oldInterfaceIndex {
		return nil
	}
	if u.interfaceName == "" {
		u.logger.Debugf("auto get interface: %s", iface.Name)
	}
	u.logger.Debug("flush dns upstream...")
	defer u.logger.Debug("flush dns upstream done")
	err = u.fetchDNSUpstream4(ctx, iface)
	if err == nil {
		u.oldInterfaceName = iface.Name
		u.oldInterfaceIndex = iface.Index
	}
	return err
}

func (u *DHCPUpstream) fetchDNSUpstream4(ctx context.Context, iface *net.Interface) error {
	listenAddr := "0.0.0.0:68"
	if runtime.GOOS == "linux" || runtime.GOOS == "android" {
		listenAddr = "255.255.255.255:68"
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return err
	}
	for _, address := range addresses {
		ipNet, ok := address.(*net.IPNet)
		if ok && ipNet.IP.To4() != nil {
			listenAddr = net.JoinHostPort(ipNet.IP.String(), "68")
			break
		}
	}
	u.listenerConfig.Control = control.AppendControl(control.BindToInterface(iface.Name, u.useIPv6), control.ReuseAddr())

	conn, err := u.listenerConfig.ListenPacket(ctx, "udp4", listenAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	discovery, err := dhcpv4.NewDiscovery(iface.HardwareAddr, dhcpv4.WithBroadcast(true), dhcpv4.WithRequestedOptions(dhcpv4.OptionDomainNameServer))
	if err != nil {
		return err
	}
	_, err = conn.WriteTo(discovery.ToBytes(), &net.UDPAddr{IP: net.IPv4bcast, Port: 67})
	if err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(DHCPDefaultRequestTimeout)
	}
	err = conn.SetReadDeadline(deadline)
	if err != nil {
		return err
	}
	pakcet := make([]byte, dhcpv4.MaxMessageSize)
	for {
		_, _, err = conn.ReadFrom(pakcet)
		if err != nil {
			return err
		}
		dhcpPacket, err := dhcpv4.FromBytes(pakcet)
		if err != nil {
			return err
		}
		if dhcpPacket.MessageType() != dhcpv4.MessageTypeOffer {
			u.logger.Debugf("unknown message type: %d", dhcpPacket.MessageType())
			continue
		}

		if dhcpPacket.TransactionID != discovery.TransactionID {
			u.logger.Debugf("unknown transaction id: %d", dhcpPacket.TransactionID)
			continue
		}

		dns := dhcpPacket.DNS()
		if len(dns) == 0 {
			return fmt.Errorf("no dns server found")
		}
		ips := make([]string, 0, len(dns))
		for _, ip := range dns {
			ips = append(ips, ip.String())
		}

		return u.flushDNSUpstream(ips)
	}
}

func (u *DHCPUpstream) flushDNSUpstream(ips []string) (err error) {
	upstreamStack := utils.NewStack[adapter.Upstream](len(ips))
	defer func() {
		if err != nil {
			for upstreamStack.Len() > 0 {
				uu := upstreamStack.Pop()
				closer, isCloser := uu.(adapter.Closer)
				if isCloser {
					err = closer.Close()
					if err != nil {
						u.logger.Errorf("close upstream[%s] failed: %s", uu.Tag(), err)
					} else {
						u.logger.Infof("close upstream[%s] success", uu.Tag())
					}
				}
			}
		}
	}()
	uus := make(map[string]adapter.Upstream, len(ips))
	add, remove := utils.Compare(u.upstreamAddresses, ips)
	if len(add) == 0 && len(remove) == 0 {
		return nil
	}
	removeMap := make(map[string]bool, len(remove))
	for _, ss := range remove {
		removeMap[ss] = true
	}
	var uu adapter.Upstream
	for _, ss := range add {
		options := Options{
			Tag:  u.tag,
			Type: UDPUpstreamType,
			UDPOptions: &UDPUpstreamOptions{
				Address:            ss,
				ConnectTimeout:     utils.Duration(u.connectTimeout),
				IdleTimeout:        utils.Duration(u.idleTimeout),
				EDNS0:              u.edns0,
				DisableFallbackTCP: u.disableFallbackTCP,
				EnablePipeline:     u.enablePipeline,
			},
		}
		uu, err = NewUpstream(u.ctx, u.core, u.logger, options.Tag, options)
		if err != nil {
			return fmt.Errorf("create dhcp item upstream failed: %s", err)
		}
		starter, isStarter := uu.(adapter.Starter)
		if isStarter {
			err = starter.Start()
			if err != nil {
				err = fmt.Errorf("start upstream[%s] failed: %s", uu.Tag(), err)
				return err
			}
		}
		upstreamStack.Push(uu)
		uus[ss] = uu
	}
	olds := u.upstreamMap
	for tag, uu := range olds {
		if _, ok := removeMap[tag]; !ok {
			uus[tag] = uu
		}
	}
	u.upstreamMap = uus
	for tag, uu := range olds {
		if _, ok := removeMap[tag]; !ok {
			continue
		}
		closer, isCloser := uu.(adapter.Closer)
		if isCloser {
			err = closer.Close()
			if err != nil {
				u.logger.Errorf("close upstream[%s] failed: %s", uu.Tag(), err)
			} else {
				u.logger.Infof("close upstream[%s] success", uu.Tag())
			}
		}
	}
	u.logger.Debugf("new upstream addresses: [%s]", strings.Join(add, ", "))
	u.upstreamAddresses = ips
	return nil
}

func (u *DHCPUpstream) Exchange(ctx context.Context, req *dns.Msg) (resp *dns.Msg, err error) {
	uus := u.upstreamMap
	if len(uus) == 0 {
		return nil, fmt.Errorf("no upstream available")
	}
	var uu adapter.Upstream
	for _, _uu := range uus {
		uu = _uu
		break
	}
	u.reqTotal.Add(1)
	resp, err = uu.Exchange(ctx, req)
	if err == nil {
		u.reqSuccess.Add(1)
	}
	return resp, err
}

func (u *DHCPUpstream) StatisticalData() map[string]any {
	total := u.reqTotal.Load()
	success := u.reqSuccess.Load()
	return map[string]any{
		"total":   total,
		"success": success,
	}
}
