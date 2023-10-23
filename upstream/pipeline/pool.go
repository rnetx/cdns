package pipeline

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const DefaultDNSPipelineConnPoolSize = 16

type DNSPipelineConnPool struct {
	ctx          context.Context
	cancel       context.CancelFunc
	closeDone    chan struct{}
	isClosed     bool
	maxSize      int
	idleTimeout  time.Duration
	connChanLock sync.RWMutex
	connChan     chan *DNSPipelineConn
	newFunc      func(ctx context.Context) (net.Conn, error)
	closeFunc    func()
	udpSize      uint16
}

func NewDNSPipelineConnPool(ctx context.Context, maxSize int, udpSize uint16, idleTimeout time.Duration, newFunc func(ctx context.Context) (net.Conn, error), closeFunc func()) *DNSPipelineConnPool {
	if maxSize <= 0 {
		maxSize = DefaultDNSPipelineConnPoolSize
	}
	ctx, cancel := context.WithCancel(ctx)
	p := &DNSPipelineConnPool{
		ctx:         ctx,
		cancel:      cancel,
		closeDone:   make(chan struct{}, 1),
		maxSize:     maxSize,
		idleTimeout: idleTimeout,
		connChan:    make(chan *DNSPipelineConn, maxSize),
		newFunc:     newFunc,
		closeFunc:   closeFunc,
		udpSize:     udpSize,
	}
	go p.loopHandle()
	return p
}

func (p *DNSPipelineConnPool) loopHandle() {
	defer func() {
		select {
		case p.closeDone <- struct{}{}:
		default:
		}
	}()
	defer p.cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.connChanLock.Lock()
			now := time.Now().UnixNano()
			connChanLen := len(p.connChan)
			for i := 0; i < connChanLen; i++ {
				select {
				case conn := <-p.connChan:
					if conn.LastUseUnix()+p.idleTimeout.Nanoseconds() > now {
						select {
						case p.connChan <- conn:
						case <-p.ctx.Done():
							p.connChanLock.Unlock()
							return
						}
						continue
					}
					conn.Close()
				case <-p.ctx.Done():
					p.connChanLock.Unlock()
					return
				}
			}
			p.connChanLock.Unlock()
		}
	}
}

func (p *DNSPipelineConnPool) Close() {
	if p.isClosed {
		return
	}
	p.isClosed = true
	p.cancel()
	<-p.closeDone
	close(p.closeDone)
	p.connChanLock.Lock()
	defer p.connChanLock.Unlock()
	for {
		select {
		case conn := <-p.connChan:
			conn.Close()
		default:
			return
		}
	}
}

func (p *DNSPipelineConnPool) Exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	if p.isClosed {
		return nil, context.Canceled
	}
	var pipelineConn *DNSPipelineConn
	p.connChanLock.RLock()
	for {
		select {
		case conn := <-p.connChan:
			if conn.IsClosed() {
				conn.Close()
				continue
			}
			pipelineConn = conn.Clone()
			select {
			case p.connChan <- conn:
			case <-p.ctx.Done():
				p.connChanLock.RUnlock()
				return nil, p.ctx.Err()
			case <-ctx.Done():
				p.connChanLock.RUnlock()
				return nil, ctx.Err()
			}
		case <-p.ctx.Done():
			p.connChanLock.RUnlock()
			return nil, p.ctx.Err()
		case <-ctx.Done():
			p.connChanLock.RUnlock()
			return nil, ctx.Err()
		default:
		}
		break
	}
	p.connChanLock.RUnlock()
	if pipelineConn == nil {
		conn, err := p.newFunc(ctx)
		if err != nil {
			return nil, err
		}
		pipelineConn = NewDNSPipelineConn(p.ctx, p.udpSize, conn, p.closeFunc)
		p.connChanLock.RLock()
		select {
		case p.connChan <- pipelineConn.Clone():
		default:
		}
		p.connChanLock.RUnlock()
	}
	defer pipelineConn.Close()
	return pipelineConn.Exchange(ctx, req)
}
