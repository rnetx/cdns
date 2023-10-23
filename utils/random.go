package utils

import (
	"crypto/rand"
	"encoding/binary"
)

func RandomIDUint64() uint64 {
	var output uint64
	err := binary.Read(rand.Reader, binary.BigEndian, &output)
	if err != nil {
		panic("reading random id failed: " + err.Error())
	}
	return output
}
