package log

import (
	"context"
	"sync"
	"time"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/utils"
)

type BroadcastMessage struct {
	Time            time.Time     `json:"time"`
	Level           Level         `json:"level"`
	Message         string        `json:"message"`
	ContextID       uint32        `json:"context_id,omitempty"`
	ContextDuration time.Duration `json:"context_duration,omitempty"`
}

type BroadcastLogger struct {
	logger basicLogger
	m      sync.Map
	Logger
}

func NewBroadcastLogger(logger Logger) *BroadcastLogger {
	s := &BroadcastLogger{
		logger: logger.basicLogger(),
	}
	s.Logger = newExportLogger(s)
	return s
}

func (s *BroadcastLogger) Close() {
	s.m.Range(func(key, value any) bool {
		ch := value.(*utils.SafeChan[BroadcastMessage])
		ch.Close()
		return true
	})
}

func (s *BroadcastLogger) level() Level {
	return s.logger.level()
}

func (s *BroadcastLogger) disableColor() bool {
	return s.logger.disableColor()
}

func (s *BroadcastLogger) print(level Level, msg string) {
	go s.sendToBroadcast(&BroadcastMessage{
		Time:    time.Now(),
		Level:   level,
		Message: msg,
	})
	s.logger.print(level, msg)
}

func (s *BroadcastLogger) printContext(ctx context.Context, level Level, msg string) {
	var (
		contextID       uint32
		contextDuration time.Duration
	)
	logContext := adapter.LoadLogContext(ctx)
	if logContext != nil {
		contextID = logContext.ID()
		contextDuration = logContext.Duration()
	}
	go s.sendToBroadcast(&BroadcastMessage{
		Time:            time.Now(),
		Level:           level,
		Message:         msg,
		ContextID:       contextID,
		ContextDuration: contextDuration,
	})
	s.logger.printContext(ctx, level, msg)
}

func (s *BroadcastLogger) sendToBroadcast(msg *BroadcastMessage) {
	s.m.Range(func(key, value any) bool {
		ctx := key.(context.Context)
		ch := value.(*utils.SafeChan[BroadcastMessage])
		select {
		case <-ctx.Done():
			s.m.Delete(key)
			ch.Close()
		case ch.SendChan() <- *msg:
		default:
			if ch.Counter() == 1 {
				s.m.Delete(key)
			}
		}
		return true
	})
}

func (s *BroadcastLogger) Register(ctx context.Context) *utils.SafeChan[BroadcastMessage] {
	ch := utils.NewSafeChan[BroadcastMessage](1)
	s.m.Store(ctx, ch)
	return ch
}

func (s *BroadcastLogger) Unregister(ctx context.Context) {
	v, ok := s.m.LoadAndDelete(ctx)
	if ok {
		ch := v.(*utils.SafeChan[BroadcastMessage])
		ch.Close()
	}
}
