package log

import (
	"context"
	"time"
)

type NopLogger struct {
	Logger
}

func NewNopLogger() Logger {
	n := &NopLogger{}
	n.Logger = newExportLogger(n)
	return n
}

func (l *NopLogger) level() Level {
	return LevelFatal
}

func (l *NopLogger) disableColor() bool {
	return true
}

func (l *NopLogger) print(_ Level, _ string) {}

func (l *NopLogger) printContext(_ context.Context, _ Level, _ string) {}

func (l *NopLogger) SetTimeFunc(_ func() time.Time) {}
