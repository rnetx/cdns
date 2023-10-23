package log

import "github.com/logrusorgru/aurora/v4"

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

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
