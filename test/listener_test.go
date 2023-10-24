package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"testing"

	"github.com/logrusorgru/aurora/v4"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/listener"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/upstream"
	"github.com/rnetx/cdns/workflow"
	"gopkg.in/yaml.v3"
)

func testListener(t *testing.T, options listener.Options, f func()) {
	ctx := simpleCore.Context()
	rootLogger := simpleCore.RootLogger()
	upstreamOptions := upstream.Options{
		Tag:  "upstream",
		Type: upstream.TCPUpstreamType,
		TCPOptions: &upstream.TCPUpstreamOptions{
			Address:        "223.5.5.5",
			EnablePipeline: true,
		},
	}
	u, err := upstream.NewUpstream(ctx, simpleCore, log.NewTagLogger(rootLogger, fmt.Sprintf("upstream/%s", upstreamOptions.Tag), aurora.GreenFg), upstreamOptions.Tag, upstreamOptions)
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
	workflowOptionsBytes := []byte(`tag: default
rules:
  - exec:
      - upstream: upstream`)
	var workflowOptions workflow.WorkflowOptions
	yaml.Unmarshal(workflowOptionsBytes, &workflowOptions)
	w, err := workflow.NewWorkflow(ctx, simpleCore, log.NewTagLogger(rootLogger, fmt.Sprintf("workflow/%s", workflowOptions.Tag), aurora.MagentaFg), workflowOptions.Tag, workflowOptions)
	if err != nil {
		t.Fatal(err)
	}
	simpleCore.AddWorkflow(w)
	defer simpleCore.RemoveWorkflow(w.Tag())
	l, err := listener.NewListener(ctx, simpleCore, log.NewTagLogger(rootLogger, fmt.Sprintf("listener/%s", options.Tag), aurora.YellowFg), options.Tag, options)
	if err != nil {
		t.Fatal(err)
	}
	simpleCore.AddListener(l)
	defer simpleCore.RemoveListener(l.Tag())
	starter, isStarter = l.(adapter.Starter)
	if isStarter {
		err := starter.Start()
		if err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		closer, isCloser := l.(adapter.Closer)
		if isCloser {
			err := closer.Close()
			if err != nil {
				t.Log(err)
			}
		}
	}()
	w.Check()
	f()
}

func TestUDPListener(t *testing.T) {
	options := listener.Options{
		Tag:      "listener",
		Type:     listener.UDPListenerType,
		Workflow: "default",
		UDPOptions: &listener.UDPListenerOptions{
			Listen: ":6053",
		},
	}
	testListener(t, options, func() {
		req := dnsRequests()[0]
		localAddr := &net.UDPAddr{IP: netip.IPv4Unspecified().AsSlice(), Zone: netip.IPv4Unspecified().Zone()}
		remoteAddr := &net.UDPAddr{IP: netip.IPv4Unspecified().AsSlice(), Zone: netip.IPv4Unspecified().Zone(), Port: 6053}

		conn, err := net.DialUDP("udp", localAddr, remoteAddr)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		dnsConn := &dns.Conn{Conn: conn}

		err = dnsConn.WriteMsg(req)
		if err != nil {
			t.Fatal(err)
		}

		_, err = dnsConn.ReadMsg()
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestTCPListener(t *testing.T) {
	options := listener.Options{
		Tag:      "listener",
		Type:     listener.TCPListenerType,
		Workflow: "default",
		TCPOptions: &listener.TCPListenerOptions{
			Listen: ":6053",
		},
	}
	testListener(t, options, func() {
		req := dnsRequests()[0]

		conn, err := net.Dial("tcp", "127.0.0.1:6053")
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		dnsConn := &dns.Conn{Conn: conn}

		err = dnsConn.WriteMsg(req)
		if err != nil {
			t.Fatal(err)
		}

		_, err = dnsConn.ReadMsg()
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestTLSListener(t *testing.T) {
	options := listener.Options{
		Tag:      "listener",
		Type:     listener.TLSListenerType,
		Workflow: "default",
		TLSOptions: &listener.TLSListenerOptions{
			Listen: ":6053",
			TLSOptions: listener.TLSOptions{
				ServerCertFile: "./server-cert.pem",
				ServerKeyFile:  "./server-key.pem",
			},
		},
	}
	testListener(t, options, func() {
		req := dnsRequests()[0]

		conn, err := tls.Dial("tcp", "127.0.0.1:6053", &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"dns"}})
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		dnsConn := &dns.Conn{Conn: conn}

		err = dnsConn.WriteMsg(req)
		if err != nil {
			t.Fatal(err)
		}

		_, err = dnsConn.ReadMsg()
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestQUICListener(t *testing.T) {
	options := listener.Options{
		Tag:      "listener",
		Type:     listener.QUICListenerType,
		Workflow: "default",
		QUICOptions: &listener.QUICListenerOptions{
			Listen: ":6053",
			TLSOptions: listener.TLSOptions{
				ServerCertFile: "./server-cert.pem",
				ServerKeyFile:  "./server-key.pem",
			},
			Enable0RTT: true,
		},
	}
	testListener(t, options, func() {
		req := dnsRequests()[0]
		rawBytes, _ := req.Pack()

		conn, err := quic.DialAddrEarly(simpleCore.Context(), "127.0.0.1:6053", &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"doq"}}, &quic.Config{Allow0RTT: true})
		if err != nil {
			t.Fatal(err)
		}
		defer conn.CloseWithError(0, "")

		stream, err := conn.OpenStreamSync(simpleCore.Context())
		if err != nil {
			t.Fatal(err)
		}

		raw := make([]byte, 2+len(rawBytes))
		binary.BigEndian.PutUint16(raw, uint16(len(rawBytes)))
		copy(raw[2:], rawBytes)

		_, err = stream.Write(raw)
		if err != nil {
			stream.Close()
			t.Fatal(err)
		}
		stream.Close()

		var length uint16
		err = binary.Read(stream, binary.BigEndian, &length)
		if err != nil {
			t.Fatal(err)
		}
		if length == 0 {
			t.Fatal("invalid length")
		}
		data := make([]byte, length)
		var n int
		n, err = stream.Read(data)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}

		resp := &dns.Msg{}
		err = resp.Unpack(data[:n])
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestHTTPListener(t *testing.T) {
	options := listener.Options{
		Tag:      "listener",
		Type:     listener.HTTPListenerType,
		Workflow: "default",
		HTTPOptions: &listener.HTTPListenerOptions{
			Listen: ":6053",
		},
	}
	testListener(t, options, func() {
		req := dnsRequests()[0]
		raw, _ := req.Pack()
		rawStr := base64.RawURLEncoding.EncodeToString(raw)

		httpReq, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:6053/dns-query?dns="+rawStr, nil)
		httpReq.Header.Set("Accept", "application/dns-message")
		httpReq.Header.Set("Content-Type", "application/dns-message")

		httpResp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			t.Fatal(err)
		}
		defer httpResp.Body.Close()

		buffer := bytes.NewBuffer(nil)
		_, err = io.Copy(buffer, httpResp.Body)
		if err != nil {
			t.Fatal(err)
		}

		resp := &dns.Msg{}
		err = resp.Unpack(buffer.Bytes())
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestHTTPListenerPOST(t *testing.T) {
	options := listener.Options{
		Tag:      "listener",
		Type:     listener.HTTPListenerType,
		Workflow: "default",
		HTTPOptions: &listener.HTTPListenerOptions{
			Listen: ":6053",
		},
	}
	testListener(t, options, func() {
		req := dnsRequests()[0]
		raw, _ := req.Pack()

		httpReq, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:6053/dns-query", bytes.NewReader(raw))
		httpReq.Header.Set("Accept", "application/dns-message")
		httpReq.Header.Set("Content-Type", "application/dns-message")

		httpResp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			t.Fatal(err)
		}
		defer httpResp.Body.Close()

		buffer := bytes.NewBuffer(nil)
		_, err = io.Copy(buffer, httpResp.Body)
		if err != nil {
			t.Fatal(err)
		}

		resp := &dns.Msg{}
		err = resp.Unpack(buffer.Bytes())
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestHTTPSListener(t *testing.T) {
	options := listener.Options{
		Tag:      "listener",
		Type:     listener.HTTPListenerType,
		Workflow: "default",
		HTTPOptions: &listener.HTTPListenerOptions{
			Listen: ":6053",
			TLSOptions: &listener.TLSOptions{
				ServerCertFile: "./server-cert.pem",
				ServerKeyFile:  "./server-key.pem",
			},
		},
	}
	testListener(t, options, func() {
		req := dnsRequests()[0]
		raw, _ := req.Pack()
		rawStr := base64.RawURLEncoding.EncodeToString(raw)

		client := &http.Client{
			Transport: &http.Transport{
				ForceAttemptHTTP2: true,
				TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2", "http/1.1", "dns"}},
			},
		}

		httpReq, _ := http.NewRequest(http.MethodGet, "https://127.0.0.1:6053/dns-query?dns="+rawStr, nil)
		httpReq.Header.Set("Accept", "application/dns-message")
		httpReq.Header.Set("Content-Type", "application/dns-message")

		httpResp, err := client.Do(httpReq)
		if err != nil {
			t.Fatal(err)
		}
		defer httpResp.Body.Close()

		buffer := bytes.NewBuffer(nil)
		_, err = io.Copy(buffer, httpResp.Body)
		if err != nil {
			t.Fatal(err)
		}

		resp := &dns.Msg{}
		err = resp.Unpack(buffer.Bytes())
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestHTTPSListenerPOST(t *testing.T) {
	options := listener.Options{
		Tag:      "listener",
		Type:     listener.HTTPListenerType,
		Workflow: "default",
		HTTPOptions: &listener.HTTPListenerOptions{
			Listen: ":6053",
			TLSOptions: &listener.TLSOptions{
				ServerCertFile: "./server-cert.pem",
				ServerKeyFile:  "./server-key.pem",
			},
		},
	}
	testListener(t, options, func() {
		req := dnsRequests()[0]
		raw, _ := req.Pack()

		client := &http.Client{
			Transport: &http.Transport{
				ForceAttemptHTTP2: true,
				TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2", "http/1.1", "dns"}},
			},
		}

		httpReq, _ := http.NewRequest(http.MethodPost, "https://127.0.0.1:6053/dns-query", bytes.NewReader(raw))
		httpReq.Header.Set("Accept", "application/dns-message")
		httpReq.Header.Set("Content-Type", "application/dns-message")

		httpResp, err := client.Do(httpReq)
		if err != nil {
			t.Fatal(err)
		}
		defer httpResp.Body.Close()

		buffer := bytes.NewBuffer(nil)
		_, err = io.Copy(buffer, httpResp.Body)
		if err != nil {
			t.Fatal(err)
		}

		resp := &dns.Msg{}
		err = resp.Unpack(buffer.Bytes())
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestHTTP3Listener(t *testing.T) {
	options := listener.Options{
		Tag:      "listener",
		Type:     listener.HTTPListenerType,
		Workflow: "default",
		HTTPOptions: &listener.HTTPListenerOptions{
			Listen:     ":6053",
			UseHTTP3:   true,
			Enable0RTT: true,
			TLSOptions: &listener.TLSOptions{
				ServerCertFile: "./server-cert.pem",
				ServerKeyFile:  "./server-key.pem",
			},
		},
	}
	testListener(t, options, func() {
		req := dnsRequests()[0]
		raw, _ := req.Pack()
		rawStr := base64.RawURLEncoding.EncodeToString(raw)

		client := &http.Client{
			Transport: &http3.RoundTripper{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					NextProtos:         []string{"h3", "dns"},
				},
			},
		}

		httpReq, _ := http.NewRequest(http.MethodGet, "https://127.0.0.1:6053/dns-query?dns="+rawStr, nil)
		httpReq.Header.Set("Accept", "application/dns-message")
		httpReq.Header.Set("Content-Type", "application/dns-message")

		httpResp, err := client.Do(httpReq)
		if err != nil {
			t.Fatal(err)
		}
		defer httpResp.Body.Close()

		buffer := bytes.NewBuffer(nil)
		_, err = io.Copy(buffer, httpResp.Body)
		if err != nil {
			t.Fatal(err)
		}

		resp := &dns.Msg{}
		err = resp.Unpack(buffer.Bytes())
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestHTTP3ListenerPOST(t *testing.T) {
	options := listener.Options{
		Tag:      "listener",
		Type:     listener.HTTPListenerType,
		Workflow: "default",
		HTTPOptions: &listener.HTTPListenerOptions{
			Listen:     ":6053",
			UseHTTP3:   true,
			Enable0RTT: true,
			TLSOptions: &listener.TLSOptions{
				ServerCertFile: "./server-cert.pem",
				ServerKeyFile:  "./server-key.pem",
			},
		},
	}
	testListener(t, options, func() {
		req := dnsRequests()[0]
		raw, _ := req.Pack()

		client := &http.Client{
			Transport: &http3.RoundTripper{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					NextProtos:         []string{"h3", "dns"},
				},
			},
		}

		httpReq, _ := http.NewRequest(http.MethodPost, "https://127.0.0.1:6053/dns-query", bytes.NewReader(raw))
		httpReq.Header.Set("Accept", "application/dns-message")
		httpReq.Header.Set("Content-Type", "application/dns-message")

		httpResp, err := client.Do(httpReq)
		if err != nil {
			t.Fatal(err)
		}
		defer httpResp.Body.Close()

		buffer := bytes.NewBuffer(nil)
		_, err = io.Copy(buffer, httpResp.Body)
		if err != nil {
			t.Fatal(err)
		}

		resp := &dns.Msg{}
		err = resp.Unpack(buffer.Bytes())
		if err != nil {
			t.Fatal(err)
		}
	})
}
