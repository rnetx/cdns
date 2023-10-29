package utils

import (
	"fmt"
	"strings"
	"unsafe"
)

func Join[T fmt.Stringer](arr []T, seq string) string {
	s := make([]string, 0, len(arr))
	for _, v := range arr {
		s = append(s, v.String())
	}
	return strings.Join(s, seq)
}

// from mosdns(https://github.com/IrineSistiana/mosdns), thank for @IrineSistiana
// BytesToStringUnsafe converts bytes to string.
func BytesToStringUnsafe(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}
