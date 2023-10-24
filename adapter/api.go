package adapter

import (
	"context"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/logrusorgru/aurora/v4"
)

type APIHandler interface {
	APIHandler() chi.Router
}

var apiLogCtxKey = (*struct{})(nil)

func SaveAPIContext(ctx context.Context, c *APILogContext) context.Context {
	return context.WithValue(ctx, apiLogCtxKey, c)
}

func LoadAPIContext(ctx context.Context) *APILogContext {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(apiLogCtxKey)
	if v == nil {
		return nil
	}
	c, ok := v.(*APILogContext)
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
