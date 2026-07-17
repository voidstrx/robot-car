package input

import (
	"encoding/binary"
	"math"
)

type InputState struct {
	Axes     [8]float32
	Buttons  uint32
	KeyCount uint8
	Keys     [16]uint16
}

func (i *InputState) Marshal() []byte {
	buf := make([]byte, 69)
	offset := 0

	for _, v := range i.Axes {
		binary.LittleEndian.PutUint32(buf[offset:offset+4], math.Float32bits(v))
		offset += 4
	}
	binary.LittleEndian.PutUint32(buf[offset:offset+4], i.Buttons)
	offset += 4

	buf[offset] = i.KeyCount
	offset++

	for j := 0; j < int(i.KeyCount) && j < len(i.Keys); j++ {
		binary.LittleEndian.PutUint16(buf[offset:offset+2], i.Keys[j])
		offset += 2
	}
	return buf[:offset]
}
