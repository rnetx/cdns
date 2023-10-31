package utils

import (
	"context"
	"sync/atomic"
)

type TaskGroup struct {
	ctx      context.Context
	n        *atomic.Int32
	doneCtx  context.Context
	doneFunc context.CancelFunc
}

func NewTaskGroupWithContext(ctx context.Context) *TaskGroup {
	g := &TaskGroup{
		ctx: ctx,
		n:   &atomic.Int32{},
	}
	g.doneCtx, g.doneFunc = context.WithCancel(g.ctx)
	g.n.Add(1)
	return g
}

func NewTaskGroup() *TaskGroup {
	return NewTaskGroupWithContext(context.Background())
}

func (g *TaskGroup) Wait() <-chan struct{} {
	if g.n.Add(-1) == 0 {
		g.doneFunc()
	}
	return g.doneCtx.Done()
}

type Task TaskGroup

func (g *TaskGroup) AddTask() *Task {
	g.n.Add(1)
	return (*Task)(g)
}

func (t *Task) Done() {
	g := (*TaskGroup)(t)
	if g.n.Add(-1) == 0 {
		g.doneFunc()
	}
}
