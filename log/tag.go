package log

import (
	"context"
	"fmt"

	"github.com/rnetx/cdns/adapter"

	"github.com/logrusorgru/aurora/v4"
)

var (
	_ basicLogger = (*TagLogger)(nil)
	_ Logger      = (*TagLogger)(nil)
)

type TagLogger struct {
	logger basicLogger
	tag    string
	color  aurora.Color
	Logger
}

func NewTagLogger(logger Logger, tag string, color aurora.Color) *TagLogger {
	t := &TagLogger{
		logger: logger.basicLogger(),
		tag:    tag,
		color:  color,
	}
	t.Logger = newExportLogger(t)
	return t
}

func (t *TagLogger) level() Level {
	return t.logger.level()
}

func (t *TagLogger) disableColor() bool {
	return t.logger.disableColor()
}

func (t *TagLogger) print(level Level, msg string) {
	if level < t.logger.level() {
		return
	}
	m := ""
	if !t.logger.disableColor() && t.color != 0 {
		m += fmt.Sprintf("[%s] ", aurora.Colorize(t.tag, t.color))
	} else {
		m += fmt.Sprintf("[%s] ", t.tag)
	}
	m += msg
	t.logger.print(level, m)
}

func (t *TagLogger) printContext(ctx context.Context, level Level, msg string) {
	if level < t.logger.level() {
		return
	}
	m := ""
	if !t.logger.disableColor() && t.color != 0 {
		m += fmt.Sprintf("[%s] ", aurora.Colorize(t.tag, t.color))
	} else {
		m += fmt.Sprintf("[%s] ", t.tag)
	}
	logContext := adapter.LoadLogContext(ctx)
	if logContext != nil {
		if !t.logger.disableColor() {
			m += fmt.Sprintf("[%s] ", aurora.Colorize(fmt.Sprintf("%d %dms", logContext.ID(), logContext.Duration().Milliseconds()), logContext.Color()))
		} else {
			m += fmt.Sprintf("[%d %dms] ", logContext.ID(), logContext.Duration().Milliseconds())
		}
	}
	m += msg
	t.logger.print(level, m)
}
