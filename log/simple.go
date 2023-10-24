package log

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/logrusorgru/aurora/v4"
	"github.com/rnetx/cdns/adapter"
)

var DefaultLogger Logger

func init() {
	DefaultLogger = NewSimpleLogger(os.Stdout, LevelInfo, false, false)
}

var (
	_ basicLogger = (*SimpleLogger)(nil)
	_ Logger      = (*SimpleLogger)(nil)
)

type SimpleLogger struct {
	writer           io.Writer
	_level           Level
	disableTimestamp bool
	_disableColor    bool
	Logger
}

func NewSimpleLogger(writer io.Writer, level Level, disableTimestamp bool, disableColor bool) Logger {
	s := &SimpleLogger{
		writer:           writer,
		_level:           level,
		disableTimestamp: disableTimestamp,
		_disableColor:    disableColor,
	}
	s.Logger = newExportLogger(s)
	return s
}

func (l *SimpleLogger) level() Level {
	return l._level
}

func (l *SimpleLogger) disableColor() bool {
	return l._disableColor
}

func (l *SimpleLogger) print(level Level, msg string) {
	if level < l._level {
		return
	}
	m := ""
	if !l.disableTimestamp {
		m += fmt.Sprintf("[%s] ", time.Now().Format(time.DateTime))
	}
	if !l._disableColor {
		m += fmt.Sprintf("[%s] ", level.ColorString())
	} else {
		m += fmt.Sprintf("[%s] ", level.String())
	}
	m += msg
	fmt.Fprintln(l.writer, m)
}

func (l *SimpleLogger) printContext(ctx context.Context, level Level, msg string) {
	if level < l._level {
		return
	}
	m := ""
	if !l.disableTimestamp {
		m += fmt.Sprintf("[%s] ", time.Now().Format(time.DateTime))
	}
	if !l._disableColor {
		m += fmt.Sprintf("[%s] ", level.ColorString())
	} else {
		m += fmt.Sprintf("[%s] ", level.String())
	}
	logContext := adapter.LoadLogContext(ctx)
	if logContext != nil {
		if !l._disableColor {
			m += fmt.Sprintf("[%s] ", aurora.Colorize(fmt.Sprintf("%d %dms", logContext.ID(), logContext.Duration().Milliseconds()), logContext.Color()))
		} else {
			m += fmt.Sprintf("[%d %dms] ", logContext.ID(), logContext.Duration().Milliseconds())
		}
	}
	m += msg
	fmt.Fprintln(l.writer, m)
}
