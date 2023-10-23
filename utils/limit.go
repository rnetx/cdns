package utils

import "context"

type Limiter struct {
	ch chan *struct{}
}

func NewLimiter(n int) *Limiter {
	l := &Limiter{
		ch: make(chan *struct{}, n),
	}
	for i := 0; i < n; i++ {
		l.ch <- (*struct{})(nil)
	}
	return l
}

func (l *Limiter) Get(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-l.ch:
		return true
	}
}

func (l *Limiter) PutBack() {
	select {
	case l.ch <- (*struct{})(nil):
	default:
	}
}
