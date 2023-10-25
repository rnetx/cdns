package pipeline

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rnetx/cdns/utils"

	"github.com/miekg/dns"
)

type DNSPipelineConn struct {
	conn      dns.Conn
	lastUse   *atomic.Int64
	n         *atomic.Int64
	chMap     *sync.Map
	ctx       context.Context
	cancel    context.CancelFunc
	closeDone chan struct{}
	isClosed  bool
	closeFunc func()
}

func NewDNSPipelineConn(ctx context.Context, udpSize uint16, conn net.Conn, closeFunc func()) *DNSPipelineConn {
	ctx, cancel := context.WithCancel(ctx)
	c := &DNSPipelineConn{
		conn:      dns.Conn{Conn: conn},
		lastUse:   &atomic.Int64{},
		n:         &atomic.Int64{},
		chMap:     &sync.Map{},
		ctx:       ctx,
		cancel:    cancel,
		closeDone: make(chan struct{}, 1),
		closeFunc: closeFunc,
	}
	if udpSize > 0 {
		c.conn.UDPSize = udpSize
	}
	c.n.Add(1)
	c.flushLastUse()
	go c.loopReadHandle()
	return c
}

func (c *DNSPipelineConn) loopReadHandle() {
	defer func() {
		select {
		case c.closeDone <- struct{}{}:
		default:
		}
	}()
	defer c.cancel()
	for {
		msg, err := c.conn.ReadMsg()
		if err != nil {
			return
		}
		v, ok := c.chMap.LoadAndDelete(msg.Id)
		if ok {
			ch := v.(*utils.SafeChan[*dns.Msg])
			select {
			case ch.SendChan() <- msg:
				ch.Close()
			case <-c.ctx.Done():
				ch.Close()
				return
			}
		}
	}
}

func (c *DNSPipelineConn) LastUseUnix() int64 {
	return c.lastUse.Load()
}

func (c *DNSPipelineConn) flushLastUse() {
	c.lastUse.Store(time.Now().UnixNano())
}

func (c *DNSPipelineConn) Clone() *DNSPipelineConn {
	c.n.Add(1)
	c.flushLastUse()
	return c
}

func (c *DNSPipelineConn) IsClosed() bool {
	return utils.IsContextCancelled(c.ctx)
}

func (c *DNSPipelineConn) close() {
	if c.isClosed {
		return
	}
	c.isClosed = true
	c.cancel()
	c.conn.Close()
	<-c.closeDone
	close(c.closeDone)
	if c.closeFunc != nil {
		c.closeFunc()
	}
	c.chMap.Range(func(key, value any) bool {
		ch := value.(*utils.SafeChan[*dns.Msg])
		ch.Close()
		return true
	})
}

func (c *DNSPipelineConn) Close() {
	if c.n.Add(-1) == 0 {
		c.close()
	}
}

func (c *DNSPipelineConn) Exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	if c.isClosed {
		return nil, context.Canceled
	}
	ch := utils.NewSafeChan[*dns.Msg](1)
	defer ch.Close()
	c.chMap.Store(req.Id, ch.Clone())
	defer c.chMap.Delete(req.Id)
	err := c.conn.WriteMsg(req)
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ctx.Done():
		return nil, context.Canceled
	case resp := <-ch.ReceiveChan():
		return resp, nil
	}
}
