package log

import (
	"context"
	"fmt"
	"time"
)

type Logger interface {
	Print(level Level, args ...interface{})
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Printf(level Level, format string, args ...interface{})
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	PrintContext(ctx context.Context, level Level, args ...interface{})
	DebugContext(ctx context.Context, args ...interface{})
	InfoContext(ctx context.Context, args ...interface{})
	WarnContext(ctx context.Context, args ...interface{})
	ErrorContext(ctx context.Context, args ...interface{})
	FatalContext(ctx context.Context, args ...interface{})
	PrintfContext(ctx context.Context, level Level, format string, args ...interface{})
	DebugfContext(ctx context.Context, format string, args ...interface{})
	InfofContext(ctx context.Context, format string, args ...interface{})
	WarnfContext(ctx context.Context, format string, args ...interface{})
	ErrorfContext(ctx context.Context, format string, args ...interface{})
	FatalfContext(ctx context.Context, format string, args ...interface{})

	basicLogger() basicLogger
}

type SetTimeFuncInterface interface {
	SetTimeFunc(func() time.Time)
}

type basicLogger interface {
	level() Level
	disableColor() bool

	print(level Level, msg string)
	printContext(ctx context.Context, level Level, msg string)
}

var _ Logger = (*ExportLogger)(nil)

type ExportLogger struct {
	logger basicLogger
}

func newExportLogger(logger basicLogger) Logger {
	return &ExportLogger{
		logger: logger,
	}
}

func (l *ExportLogger) basicLogger() basicLogger {
	return l.logger
}

func (l *ExportLogger) Print(level Level, args ...interface{}) {
	l.logger.print(level, fmt.Sprint(args...))
}

func (l *ExportLogger) Printf(level Level, format string, args ...interface{}) {
	l.logger.print(level, fmt.Sprintf(format, args...))
}

func (l *ExportLogger) PrintContext(ctx context.Context, level Level, args ...interface{}) {
	l.logger.printContext(ctx, level, fmt.Sprint(args...))
}

func (l *ExportLogger) PrintfContext(ctx context.Context, level Level, format string, args ...interface{}) {
	l.logger.printContext(ctx, level, fmt.Sprintf(format, args...))
}

func (l *ExportLogger) Debug(args ...interface{}) {
	l.Print(LevelDebug, args...)
}

func (l *ExportLogger) Info(args ...interface{}) {
	l.Print(LevelInfo, args...)
}

func (l *ExportLogger) Warn(args ...interface{}) {
	l.Print(LevelWarn, args...)
}

func (l *ExportLogger) Error(args ...interface{}) {
	l.Print(LevelError, args...)
}

func (l *ExportLogger) Fatal(args ...interface{}) {
	l.Print(LevelFatal, args...)
}

func (l *ExportLogger) Debugf(format string, args ...interface{}) {
	l.Printf(LevelDebug, format, args...)
}

func (l *ExportLogger) Infof(format string, args ...interface{}) {
	l.Printf(LevelInfo, format, args...)
}

func (l *ExportLogger) Warnf(format string, args ...interface{}) {
	l.Printf(LevelWarn, format, args...)
}

func (l *ExportLogger) Errorf(format string, args ...interface{}) {
	l.Printf(LevelError, format, args...)
}

func (l *ExportLogger) Fatalf(format string, args ...interface{}) {
	l.Printf(LevelFatal, format, args...)
}

func (l *ExportLogger) DebugContext(ctx context.Context, args ...interface{}) {
	l.PrintContext(ctx, LevelDebug, args...)
}

func (l *ExportLogger) InfoContext(ctx context.Context, args ...interface{}) {
	l.PrintContext(ctx, LevelInfo, args...)
}

func (l *ExportLogger) WarnContext(ctx context.Context, args ...interface{}) {
	l.PrintContext(ctx, LevelWarn, args...)
}

func (l *ExportLogger) ErrorContext(ctx context.Context, args ...interface{}) {
	l.PrintContext(ctx, LevelError, args...)
}

func (l *ExportLogger) FatalContext(ctx context.Context, args ...interface{}) {
	l.PrintContext(ctx, LevelFatal, args...)
}

func (l *ExportLogger) DebugfContext(ctx context.Context, format string, args ...interface{}) {
	l.PrintfContext(ctx, LevelDebug, format, args...)
}

func (l *ExportLogger) InfofContext(ctx context.Context, format string, args ...interface{}) {
	l.PrintfContext(ctx, LevelInfo, format, args...)
}

func (l *ExportLogger) WarnfContext(ctx context.Context, format string, args ...interface{}) {
	l.PrintfContext(ctx, LevelWarn, format, args...)
}

func (l *ExportLogger) ErrorfContext(ctx context.Context, format string, args ...interface{}) {
	l.PrintfContext(ctx, LevelError, format, args...)
}

func (l *ExportLogger) FatalfContext(ctx context.Context, format string, args ...interface{}) {
	l.PrintfContext(ctx, LevelFatal, format, args...)
}
