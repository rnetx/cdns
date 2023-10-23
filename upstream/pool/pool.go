package pool

import (
	"context"
	"errors"
	"sync"
	"time"
)

const DefaultPoolMaxSize = 16

type Item[T any] struct {
	v       T
	lastUse time.Time
}

type Pool[T any] struct {
	ctx          context.Context
	cancel       context.CancelFunc
	isClosed     bool
	closeDone    chan struct{}
	idleTimeout  time.Duration
	maxSize      int
	connChanLock sync.RWMutex
	connChan     chan Item[T]
	newFunc      func(ctx context.Context) (T, error)
	closeFunc    func(T)
}

func NewPool[T any](ctx context.Context, maxSize int, idleTimeout time.Duration, newFunc func(ctx context.Context) (T, error), closeFunc func(T)) *Pool[T] {
	if maxSize == 0 {
		maxSize = DefaultPoolMaxSize
	}
	ctx, cancel := context.WithCancel(ctx)
	p := &Pool[T]{
		ctx:         ctx,
		cancel:      cancel,
		closeDone:   make(chan struct{}, 1),
		idleTimeout: idleTimeout,
		maxSize:     maxSize,
		connChan:    make(chan Item[T], maxSize),
		newFunc:     newFunc,
		closeFunc:   closeFunc,
	}
	go p.loopHandle()
	return p
}

func (p *Pool[T]) loopHandle() {
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
			now := time.Now()
			connChanLen := len(p.connChan)
			for i := 0; i < connChanLen; i++ {
				select {
				case item := <-p.connChan:
					if now.Sub(item.lastUse) < p.idleTimeout {
						select {
						case p.connChan <- item:
						case <-p.ctx.Done():
							p.connChanLock.Unlock()
							return
						}
						continue
					}
					if p.closeFunc != nil {
						p.closeFunc(item.v)
					}
				case <-p.ctx.Done():
					p.connChanLock.Unlock()
					return
				}
			}
			p.connChanLock.Unlock()
		}
	}
}

func (p *Pool[T]) Close() {
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
		case item := <-p.connChan:
			if p.closeFunc != nil {
				p.closeFunc(item.v)
			}
		default:
		}
		break
	}
	close(p.connChan)
}

func (p *Pool[T]) Get(ctx context.Context) (T, error) {
	if p.isClosed {
		var v T
		return v, p.ctx.Err()
	}
	var ok bool
	var v T
	p.connChanLock.RLock()
	for {
		select {
		case item := <-p.connChan:
			v = item.v
			ok = true
		case <-ctx.Done():
			p.connChanLock.RUnlock()
			return v, ctx.Err()
		case <-p.ctx.Done():
			p.connChanLock.RUnlock()
			return v, p.ctx.Err()
		default:
		}
		break
	}
	p.connChanLock.RUnlock()
	if !ok {
		var err error
		v, err = p.newFunc(ctx)
		if err != nil {
			return v, err
		}
	}
	return v, nil
}

func (p *Pool[T]) Put(ctx context.Context, v T) error {
	if p.isClosed {
		return p.ctx.Err()
	}
	var err error
	p.connChanLock.RLock()
	select {
	case p.connChan <- Item[T]{v: v, lastUse: time.Now()}:
	case <-ctx.Done():
		err = ctx.Err()
	case <-p.ctx.Done():
		err = p.ctx.Err()
	default:
		err = errors.New("pool is full")
	}
	p.connChanLock.RUnlock()
	if err == nil {
		return nil
	}
	if p.closeFunc != nil {
		p.closeFunc(v)
	}
	return err
}
