package utils

import "sync/atomic"

type SafeChan[T any] struct {
	ch chan T
	n  *atomic.Int32
}

func NewSafeChan[T any](size int) *SafeChan[T] {
	c := &SafeChan[T]{
		n: &atomic.Int32{},
	}
	c.n.Add(1)
	if size == 0 {
		c.ch = make(chan T)
	} else {
		c.ch = make(chan T, size)
	}
	return c
}

func (c *SafeChan[T]) Counter() int {
	return int(c.n.Load())
}

func (c *SafeChan[T]) Clone() *SafeChan[T] {
	c.n.Add(1)
	return c
}

func (c *SafeChan[T]) ReceiveChan() <-chan T {
	return c.ch
}

func (c *SafeChan[T]) SendChan() chan<- T {
	return c.ch
}

func (c *SafeChan[T]) Close() {
	if c.n.Add(-1) == 0 {
		close(c.ch)
	}
}
