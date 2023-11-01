package main

import (
	"context"
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/upstream"
	"github.com/rnetx/cdns/utils"
	"github.com/rnetx/cdns/utils/network"
	"github.com/rnetx/cdns/utils/network/socks5"

	"github.com/logrusorgru/aurora/v4"
	"github.com/miekg/dns"
)

var domains = []string{
	"www.baidu.com",
	"www.360.com",
	"www.qq.com",
	"www.taobao.com",
	"www.jd.com",
	"www.tmall.com",
	"www.sina.com.cn",
	"www.sohu.com",
}

func dnsRequests() []*dns.Msg {
	s := make([]*dns.Msg, 0, len(domains))
	for _, domain := range domains {
		msg := &dns.Msg{}
		msg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
		s = append(s, msg)
	}
	return s
}

func reqInfo(req *dns.Msg) string {
	return fmt.Sprintf("%s %s %s", req.Question[0].Name, dns.TypeToString[req.Question[0].Qtype], dns.ClassToString[req.Question[0].Qclass])
}

func initTestUpstream(t *testing.T, options upstream.Options) {
	ctx := simpleCore.Context()
	rootLogger := simpleCore.RootLogger()
	u, err := upstream.NewUpstream(ctx, simpleCore, log.NewTagLogger(rootLogger, fmt.Sprintf("upstream/%s", options.Tag), aurora.GreenFg), options.Tag, options)
	if err != nil {
		t.Fatal(err)
	}
	simpleCore.AddUpstream(u)
	defer simpleCore.RemoveUpstream(u.Tag())
	starter, isStarter := u.(adapter.Starter)
	if isStarter {
		err := starter.Start()
		if err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		closer, isCloser := u.(adapter.Closer)
		if isCloser {
			err := closer.Close()
			if err != nil {
				t.Log(err)
			}
		}
	}()
	testExchange(ctx, t, u, rootLogger)
}

func testExchange(ctx context.Context, t *testing.T, u adapter.Upstream, rootLogger log.Logger) {
	reqs := dnsRequests()
	ch := utils.NewSafeChan[struct{}](len(reqs))
	defer ch.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, req := range reqs {
		time.Sleep(10 * time.Millisecond) // Sleep 10ms
		go func(ch *utils.SafeChan[struct{}], req *dns.Msg) {
			defer func() {
				select {
				case ch.SendChan() <- struct{}{}:
				case <-ctx.Done():
				}
				ch.Close()
			}()
			rootLogger.Infof("exchange %s", reqInfo(req))
			start := time.Now()
			_, err := u.Exchange(ctx, req)
			if err != nil {
				rootLogger.Errorf("exchange %s error: %s, cost: %s", reqInfo(req), err, time.Since(start))
			} else {
				rootLogger.Infof("exchange %s success, cost: %s", reqInfo(req), time.Since(start))
			}
		}(ch.Clone(), req)
	}
	for i := 0; i < len(reqs); i++ {
		select {
		case <-ch.ReceiveChan():
		case <-ctx.Done():
			return
		}
	}
}

func TestUDPUpstream(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.UDPUpstreamType,
		UDPOptions: &upstream.UDPUpstreamOptions{
			Address: "223.5.5.5",
		},
	}
	initTestUpstream(t, options)
}

func TestUDPUpstreamSocks5(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.UDPUpstreamType,
		UDPOptions: &upstream.UDPUpstreamOptions{
			Address: "223.5.5.5",
			DialerOptions: network.Options{
				Socks5Options: &socks5.Options{
					Address: netip.MustParseAddrPort("127.0.0.1:1080"),
				},
			},
		},
	}
	initTestUpstream(t, options)
}

func TestTCPUpstream(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.TCPUpstreamType,
		TCPOptions: &upstream.TCPUpstreamOptions{
			Address: "223.5.5.5",
		},
	}
	initTestUpstream(t, options)
}

func TestTCPUpstreamPipeline(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.TCPUpstreamType,
		TCPOptions: &upstream.TCPUpstreamOptions{
			Address:        "223.5.5.5",
			EnablePipeline: true,
		},
	}
	initTestUpstream(t, options)
}

func TestTCPUpstreamSocks5(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.TCPUpstreamType,
		TCPOptions: &upstream.TCPUpstreamOptions{
			Address: "223.5.5.5",
			DialerOptions: network.Options{
				Socks5Options: &socks5.Options{
					Address: netip.MustParseAddrPort("127.0.0.1:1080"),
				},
			},
		},
	}
	initTestUpstream(t, options)
}

func TestTLSUpstream(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.TLSUpstreamType,
		TLSOptions: &upstream.TLSUpstreamOptions{
			Address: "223.5.5.5",
		},
	}
	initTestUpstream(t, options)
}

func TestTLSUpstreamPipeline(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.TLSUpstreamType,
		TLSOptions: &upstream.TLSUpstreamOptions{
			Address:        "223.5.5.5",
			EnablePipeline: true,
		},
	}
	initTestUpstream(t, options)
}

func TestTLSUpstreamSocks5(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.TLSUpstreamType,
		TLSOptions: &upstream.TLSUpstreamOptions{
			Address: "223.5.5.5",
			DialerOptions: network.Options{
				Socks5Options: &socks5.Options{
					Address: netip.MustParseAddrPort("127.0.0.1:1080"),
				},
			},
		},
	}
	initTestUpstream(t, options)
}

func TestQUICUpstream(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.QUICUpstreamType,
		QUICOptions: &upstream.QUICUpstreamOptions{
			Address: "94.140.14.14:784",
			TLSOptions: upstream.TLSOptions{
				Servername: "dns.adguard-dns.com",
			},
		},
	}
	initTestUpstream(t, options)
}

func TestQUICUpstreamSocks5(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.QUICUpstreamType,
		QUICOptions: &upstream.QUICUpstreamOptions{
			Address: "223.5.5.5",
			DialerOptions: network.Options{
				Socks5Options: &socks5.Options{
					Address: netip.MustParseAddrPort("127.0.0.1:1080"),
				},
			},
		},
	}
	initTestUpstream(t, options)
}

func TestHTTPSUpstream(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.HTTPSUpstreamType,
		HTTPSOptions: &upstream.HTTPSUpstreamOptions{
			Address: "223.5.5.5",
		},
	}
	initTestUpstream(t, options)
}

func TestHTTPSUpstreamSocks5(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.HTTPSUpstreamType,
		HTTPSOptions: &upstream.HTTPSUpstreamOptions{
			Address: "223.5.5.5",
			DialerOptions: network.Options{
				Socks5Options: &socks5.Options{
					Address: netip.MustParseAddrPort("127.0.0.1:1080"),
				},
			},
		},
	}
	initTestUpstream(t, options)
}

func TestHTTP3Upstream(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.HTTPSUpstreamType,
		HTTPSOptions: &upstream.HTTPSUpstreamOptions{
			Address:  "223.5.5.5",
			UseHTTP3: true,
		},
	}
	initTestUpstream(t, options)
}

func TestHTTP3UpstreamSocks5(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.HTTPSUpstreamType,
		HTTPSOptions: &upstream.HTTPSUpstreamOptions{
			Address:  "223.5.5.5",
			UseHTTP3: true,
			DialerOptions: network.Options{
				Socks5Options: &socks5.Options{
					Address: netip.MustParseAddrPort("127.0.0.1:1080"),
				},
			},
		},
	}
	initTestUpstream(t, options)
}

func TestHTTPSUpstreamPOST(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.HTTPSUpstreamType,
		HTTPSOptions: &upstream.HTTPSUpstreamOptions{
			Address: "223.5.5.5",
			UsePost: true,
		},
	}
	initTestUpstream(t, options)
}

func TestHTTPSUpstreamSocks5POST(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.HTTPSUpstreamType,
		HTTPSOptions: &upstream.HTTPSUpstreamOptions{
			Address: "223.5.5.5",
			UsePost: true,
			DialerOptions: network.Options{
				Socks5Options: &socks5.Options{
					Address: netip.MustParseAddrPort("127.0.0.1:1080"),
				},
			},
		},
	}
	initTestUpstream(t, options)
}

func TestHTTP3UpstreamPOST(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.HTTPSUpstreamType,
		HTTPSOptions: &upstream.HTTPSUpstreamOptions{
			Address:  "223.5.5.5",
			UsePost:  true,
			UseHTTP3: true,
		},
	}
	initTestUpstream(t, options)
}

func TestHTTP3UpstreamSocks5POST(t *testing.T) {
	options := upstream.Options{
		Tag:  "upstream",
		Type: upstream.HTTPSUpstreamType,
		HTTPSOptions: &upstream.HTTPSUpstreamOptions{
			Address:  "223.5.5.5",
			UsePost:  true,
			UseHTTP3: true,
			DialerOptions: network.Options{
				Socks5Options: &socks5.Options{
					Address: netip.MustParseAddrPort("127.0.0.1:1080"),
				},
			},
		},
	}
	initTestUpstream(t, options)
}
