//go:build !(windows || linux || darwin)

package ntp

import "time"

func SetSystemTime(nowTime time.Time) error {
	return nil
}
