package input

import (
	"encoding/binary"
	"io"
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

func (i *InputState) Unmarshal(data []byte) error {
	if len(data) < 13 {
		return io.ErrUnexpectedEOF
	}
	offset := 0

	for j := range i.Axes {
		i.Axes[j] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
		offset += 4
	}
	i.Buttons = binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	i.KeyCount = data[offset]
	offset++

	for j := 0; j < int(i.KeyCount) && offset+2 <= len(data); j++ {
		i.Keys[j] = binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2
	}
	return nil
}
