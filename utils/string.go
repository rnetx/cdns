package utils

import (
	"fmt"
	"strings"
)

func Join[T fmt.Stringer](arr []T, seq string) string {
	s := make([]string, 0, len(arr))
	for _, v := range arr {
		s = append(s, v.String())
	}
	return strings.Join(s, seq)
}
