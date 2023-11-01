package control

import (
	"errors"
	"strings"
	"syscall"
)

func AppendControl(c ...func(network string, address string, c syscall.RawConn) error) func(network string, address string, c syscall.RawConn) error {
	return func(network string, address string, rawConn syscall.RawConn) error {
		if c != nil {
			var err error
			errs := make([]string, 0)
			for _, f := range c {
				err = f(network, address, rawConn)
				if err != nil {
					errs = append(errs, err.Error())
				}
			}
			if len(errs) > 0 {
				return errors.New(strings.Join(errs, ", and "))
			}
		}
		return nil
	}
}
