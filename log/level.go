package log

import (
	"fmt"

	"github.com/logrusorgru/aurora/v4"
)

type Level int

func (l Level) MarshalText() ([]byte, error) {
	return []byte(l.LowString()), nil
}

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

func (l Level) LowString() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "Debug"
	case LevelInfo:
		return "Info"
	case LevelWarn:
		return "Warn"
	case LevelError:
		return "Error"
	case LevelFatal:
		return "Fatal"
	default:
		return "Unknown"
	}
}

func (l Level) ColorString() string {
	switch l {
	case LevelDebug:
		return aurora.Blue("Debug").String()
	case LevelInfo:
		return aurora.Green("Info").String()
	case LevelWarn:
		return aurora.Yellow("Warn").String()
	case LevelError:
		return aurora.Red("Error").String()
	case LevelFatal:
		return aurora.Magenta("Fatal").String()
	default:
		return "Unknown"
	}
}

func ParseLevelString(s string) (Level, error) {
	var level Level
	switch s {
	case "debug", "Debug":
		level = LevelDebug
	case "info", "Info":
		level = LevelInfo
	case "warn", "Warn", "warning", "Warning":
		level = LevelWarn
	case "error", "Error":
		level = LevelError
	case "fatal", "Fatal":
		level = LevelFatal
	default:
		return 0, fmt.Errorf("invalid level: %s", s)
	}
	return level, nil
}
