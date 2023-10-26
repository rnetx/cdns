package utils

import (
	"context"
	"sync/atomic"
)

type TaskGroup struct {
	ctx    context.Context
	cancel context.CancelFunc
	n      *atomic.Int64
}

type Task TaskGroup

func NewTaskGroup() *TaskGroup {
	ctx, cancel := context.WithCancel(context.Background())
	return &TaskGroup{
		ctx:    ctx,
		cancel: cancel,
		n:      new(atomic.Int64),
	}
}

func NewTaskGroupWithContext(ctx context.Context) *TaskGroup {
	ctx, cancel := context.WithCancel(ctx)
	return &TaskGroup{
		ctx:    ctx,
		cancel: cancel,
		n:      new(atomic.Int64),
	}
}

func (t *TaskGroup) AddTask() *Task {
	t.n.Add(1)
	return (*Task)(t)
}

func (t *Task) Finish() {
	if t.n.Add(-1) <= 0 {
		t.cancel()
	}
}

func (t *TaskGroup) Wait() <-chan struct{} {
	return t.ctx.Done()
}

func (t *TaskGroup) Close() {
	t.cancel()
}
