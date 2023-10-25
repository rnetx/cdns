package memcache

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	t, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(t)
	return nil
}

type Item[T any] struct {
	Value    T         `json:"value"`
	TTL      Duration  `json:"ttl"`
	Deadline time.Time `json:"-"`
}

type CacheMap[T any] struct {
	ctx       context.Context
	cancel    context.CancelFunc
	m         map[string]*Item[T]
	lock      sync.RWMutex
	closeDone chan struct{}
}

func NewCacheMap[T any](ctx context.Context) *CacheMap[T] {
	ctx, cancel := context.WithCancel(ctx)
	return &CacheMap[T]{
		ctx:       ctx,
		cancel:    cancel,
		m:         make(map[string]*Item[T]),
		closeDone: make(chan struct{}, 1),
	}
}

func (m *CacheMap[T]) Start() {
	go m.loopHandle()
}

func (m *CacheMap[T]) Close() {
	m.cancel()
	<-m.closeDone
	close(m.closeDone)
}

func (m *CacheMap[T]) loopHandle() {
	defer func() {
		select {
		case m.closeDone <- struct{}{}:
		default:
		}
	}()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.lock.Lock()
			for k, v := range m.m {
				if time.Now().After(v.Deadline) {
					delete(m.m, k)
				}
			}
			m.lock.Unlock()
		}
	}
}

func (m *CacheMap[T]) Get(key string) (T, bool, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	var (
		v         T
		isExpired bool
	)
	item, ok := m.m[key]
	if ok {
		v = item.Value
		isExpired = time.Now().After(item.Deadline)
	}
	return v, isExpired, ok
}

func (m *CacheMap[T]) Set(key string, value T, ttl time.Duration) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.m[key] = &Item[T]{
		Value:    value,
		TTL:      Duration(ttl),
		Deadline: time.Now().Add(ttl),
	}
}

func (m *CacheMap[T]) Delete(key string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.m, key)
}

func (m *CacheMap[T]) FlushAll() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.m = make(map[string]*Item[T])
}

func (m *CacheMap[T]) Encode() ([]byte, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return json.Marshal(m.m)
}

func Decode[T any](ctx context.Context, raw []byte) (*CacheMap[T], error) {
	var mm map[string]*Item[T]
	err := json.Unmarshal(raw, &mm)
	if err != nil {
		return nil, err
	}
	for _, item := range mm {
		item.Deadline = time.Now().Add(time.Duration(item.TTL))
	}
	ctx, cancel := context.WithCancel(ctx)
	m := &CacheMap[T]{
		ctx:       ctx,
		cancel:    cancel,
		m:         mm,
		closeDone: make(chan struct{}, 1),
	}
	return m, nil
}
