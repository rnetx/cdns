package adapter

type ReturnMode int

const (
	ReturnModeUnknown ReturnMode = iota
	ReturnModeContinue
	ReturnModeReturnAll
	ReturnModeReturnOnce
)

func (r ReturnMode) String() string {
	switch r {
	case ReturnModeUnknown:
		return "unknown"
	case ReturnModeContinue:
		return "continue"
	case ReturnModeReturnAll:
		return "return all"
	case ReturnModeReturnOnce:
		return "return once"
	default:
		return "unknown"
	}
}
