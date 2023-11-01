package ntp

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/utils"
	"github.com/rnetx/cdns/utils/network"
	"github.com/rnetx/cdns/utils/network/basic"
	"github.com/rnetx/cdns/utils/network/common"

	"github.com/beevik/ntp"
	"github.com/miekg/dns"
)

type NTPOptions struct {
	Server             string         `yaml:"server"`
	Interval           utils.Duration `yaml:"interval"`
	Upstream           string         `yaml:"upstream"`
	WriteToSystem      bool           `yaml:"write-to-system"`
	BasicDialerOptions basic.Options  `yaml:",inline"`
}

const DefaultUpdateInterval = 30 * time.Minute

type NTPServer struct {
	ctx    context.Context
	core   adapter.Core
	logger log.Logger

	server        common.SocksAddr
	dialer        common.Dialer
	interval      time.Duration
	writeToSystem bool

	loopCtx         context.Context
	loopCancel      context.CancelFunc
	closeDone       chan struct{}
	ips             []netip.Addr
	ipCacheDeadline time.Time

	upstreamTag string
	upstream    adapter.Upstream
	clockOffset time.Duration
}

type rootLogger interface {
	RootLogger() log.Logger
}

func NewNTPServer(ctx context.Context, core adapter.Core, logger log.Logger, options NTPOptions) (*NTPServer, error) {
	s := &NTPServer{
		ctx:    ctx,
		core:   core,
		logger: logger,
	}
	socksAddr, err := common.NewSocksAddrFromStringWithDefaultPort(options.Server, 123)
	if err != nil {
		return nil, fmt.Errorf("create ntp server failed: %w", err)
	}
	s.server = *socksAddr
	dialer, err := basic.NewDialer(options.BasicDialerOptions)
	if err != nil {
		return nil, fmt.Errorf("create ntp dialer failed: %w", err)
	}
	s.dialer = dialer
	if options.Interval > 0 {
		s.interval = time.Duration(options.Interval)
	} else {
		s.interval = DefaultUpdateInterval
	}
	if s.server.IsDomain() {
		if options.Upstream == "" {
			return nil, fmt.Errorf("create ntp server failed: upstream is required when server is domain address")
		}
		s.upstreamTag = options.Upstream
	}
	return s, nil
}

func (s *NTPServer) Start() error {
	if s.upstreamTag != "" {
		u := s.core.GetUpstream(s.upstreamTag)
		if u == nil {
			return fmt.Errorf("upstream [%s] not found", s.upstreamTag)
		}
		s.upstream = u
	}
	err := s.update(s.ctx)
	if err != nil {
		return fmt.Errorf("update ntp failed: %w", err)
	} else {
		offset := s.clockOffset
		s.logger.Infof("update ntp success, time now: %s, offset: %s", time.Now().Add(offset).Format(time.DateTime), offset.String())
	}
	s.loopCtx, s.loopCancel = context.WithCancel(s.ctx)
	s.closeDone = make(chan struct{}, 1)
	go s.loopUpdate()
	return nil
}

func (s *NTPServer) Close() error {
	s.loopCancel()
	<-s.closeDone
	return nil
}

func (s *NTPServer) loopUpdate() {
	defer func() {
		select {
		case s.closeDone <- struct{}{}:
		default:
		}
	}()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.loopCtx.Done():
			return
		case <-ticker.C:
			err := s.update(s.loopCtx)
			if err != nil {
				s.logger.Errorf("update ntp failed: %v", err)
			} else {
				s.logger.Infof("update ntp success, time now: %s", time.Now().Add(s.clockOffset).Format(time.DateTime))
			}
		}
	}
}

func (s *NTPServer) newConn(ctx context.Context) (conn net.Conn, err error) {
	socksAddr := s.server
	if socksAddr.IsDomain() && (len(s.ips) == 0 || s.ipCacheDeadline.Before(time.Now())) {
		reqMsg := &dns.Msg{}
		reqMsg.SetQuestion(dns.Fqdn(socksAddr.Domain()), dns.TypeA)
		respMsg, err := s.upstream.Exchange(ctx, reqMsg)
		if err != nil {
			return nil, err
		}
		ips := make([]netip.Addr, 0)
		var minTTL uint32
		for _, ans := range respMsg.Answer {
			switch rr := ans.(type) {
			case *dns.A:
				ttl := rr.Header().Ttl
				if minTTL == 0 || (ttl != 0 && ttl < minTTL) {
					minTTL = ttl
				}
				ip, ok := netip.AddrFromSlice(rr.A)
				if ok {
					ips = append(ips, ip)
				}
			case *dns.AAAA:
				ttl := rr.Header().Ttl
				if minTTL == 0 || (ttl != 0 && ttl < minTTL) {
					minTTL = ttl
				}
				ip, ok := netip.AddrFromSlice(rr.AAAA)
				if ok {
					ips = append(ips, ip)
				}
			}
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no ips found")
		}
		ttl := time.Duration(minTTL) / time.Second
		if ttl == 0 {
			ttl = 5 * time.Minute
		}
		conn, _, err = network.DialParallel(ctx, s.dialer, "udp", ips, socksAddr.Port())
		if err != nil {
			return nil, err
		}
		s.ips = ips
		s.ipCacheDeadline = time.Now().Add(ttl)
	} else {
		var err error
		if len(s.ips) > 0 {
			conn, _, err = network.DialParallel(ctx, s.dialer, "udp", s.ips, socksAddr.Port())
		} else {
			conn, err = s.dialer.DialContext(ctx, "udp", socksAddr)
		}
		if err != nil {
			return nil, err
		}
	}
	return conn, nil
}

func (s *NTPServer) update(ctx context.Context) error {
	resp, err := ntp.QueryWithOptions(s.server.String(), ntp.QueryOptions{
		Dialer: func(_, _ string) (net.Conn, error) {
			return s.newConn(ctx)
		},
	})
	if err != nil {
		return err
	}
	s.clockOffset = resp.ClockOffset
	if s.writeToSystem {
		err := SetSystemTime(time.Now().Add(resp.ClockOffset))
		if err != nil {
			s.logger.Errorf("set system time failed: %v", err)
		}
	}
	// Maybe Panic
	s.core.(rootLogger).RootLogger().(log.SetTimeFuncInterface).SetTimeFunc(s.TimeFunc())
	return nil
}

func (s *NTPServer) TimeFunc() func() time.Time {
	return func() time.Time {
		return time.Now().Add(s.clockOffset)
	}
}
