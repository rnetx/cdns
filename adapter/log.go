package adapter

import (
	"context"
	"time"

	"github.com/logrusorgru/aurora/v4"
)

type LogContext interface {
	ID() uint32
	Color() aurora.Color
	Duration() time.Duration
}

var logCtxKey = (*struct{})(nil)

func SaveLogContext(ctx context.Context, logContext LogContext) context.Context {
	return context.WithValue(ctx, logCtxKey, logContext)
}

func LoadLogContext(ctx context.Context) LogContext {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(logCtxKey)
	if v == nil {
		return nil
	}
	c, ok := v.(LogContext)
	if ok {
		return c
	} else {
		return nil
	}
}

type APILogContext struct {
	initTime time.Time
	id       uint32
	color    aurora.Color
}

func NewAPILogContext() *APILogContext {
	return &APILogContext{
		initTime: time.Now(),
		id:       randomID(),
		color:    idToColor(randomID()),
	}
}

func (c *APILogContext) ID() uint32 {
	return c.id
}

func (c *APILogContext) Color() aurora.Color {
	return c.color
}

func (c *APILogContext) Duration() time.Duration {
	return time.Since(c.initTime)
}
