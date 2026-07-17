package i2c

import (
	"github.com/d2r2/go-i2c"
)

// I2CBus wraps go-i2c for reuse (future MPU etc).
type I2CBus struct {
	bus *i2c.I2C
}

func NewI2CBus(busNum int, addr uint8) (*I2CBus, error) {
	b, err := i2c.NewI2C(addr, busNum)
	if err != nil {
		return nil, err
	}
	return &I2CBus{bus: b}, nil
}

func (b *I2CBus) WriteRegU8(reg uint8, data byte) error {
	return b.bus.WriteRegU8(reg, data)
}

func (b *I2CBus) ReadRegU8(reg uint8) (byte, error) {
	return b.bus.ReadRegU8(reg)
}

func (b *I2CBus) Close() error {
	return b.bus.Close()
}

// For direct PCA access if needed, but we use in pca9685.go
