package utils

import (
	"crypto/rand"
	"encoding/binary"
)

func RandomIDUint16() uint16 {
	var output uint16
	err := binary.Read(rand.Reader, binary.BigEndian, &output)
	if err != nil {
		panic("reading random id failed: " + err.Error())
	}
	return output
}
