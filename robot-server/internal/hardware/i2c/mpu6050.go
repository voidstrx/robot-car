package i2c

import (
	"fmt"
	"time"

	"github.com/d2r2/go-i2c"
)

const (
	MPU6050_WHO_AM_I     = 0x75
	MPU6050_PWR_MGMT_1   = 0x6B
	MPU6050_ACCEL_XOUT_H = 0x3B
	MPU6050_GYRO_XOUT_H  = 0x43
)

type MPU6050 struct {
	bus      *i2c.I2C
	addr     uint8
	gxOffset float64
	gyOffset float64
	gzOffset float64
}

func NewMPU6050(busNum int, addr uint8) (*MPU6050, error) {
	bus, err := i2c.NewI2C(addr, busNum)
	if err != nil {
		return nil, fmt.Errorf("i2c.NewI2C: %w", err)
	}

	m := &MPU6050{bus: bus, addr: addr}

	m.writeByte(MPU6050_PWR_MGMT_1, 0x00)
	time.Sleep(100 * time.Millisecond)

	who, _ := m.readByte(MPU6050_WHO_AM_I)
	if who != 0x68 && who != 0x70 {
		bus.Close()
		return nil, fmt.Errorf("MPU6050 не найден (whoami=0x%02X)", who)
	}

	m.calibrateGyro()
	fmt.Printf("[MPU-6050] Инициализирован (адрес 0x%X)\n", addr)
	return m, nil
}

func (m *MPU6050) Close() error {
	return m.bus.Close()
}

func (m *MPU6050) calibrateGyro() {
	const samples = 100
	var sumX, sumY, sumZ float64

	for i := 0; i < samples; i++ {
		gx, gy, gz, _ := m.ReadGyroRaw()
		sumX += gx
		sumY += gy
		sumZ += gz
		time.Sleep(10 * time.Millisecond)
	}

	m.gxOffset = sumX / samples
	m.gyOffset = sumY / samples
	m.gzOffset = sumZ / samples
}

func (m *MPU6050) ReadAccel() (x, y, z float64) {
	axRaw, _ := m.readWord(MPU6050_ACCEL_XOUT_H)
	ayRaw, _ := m.readWord(MPU6050_ACCEL_XOUT_H + 2)
	azRaw, _ := m.readWord(MPU6050_ACCEL_XOUT_H + 4)

	x = float64(int16(axRaw)) / 16384.0
	y = float64(int16(ayRaw)) / 16384.0
	z = float64(int16(azRaw)) / 16384.0
	return
}

func (m *MPU6050) ReadGyroRaw() (x, y, z float64, err error) {
	gxRaw, err := m.readWord(MPU6050_GYRO_XOUT_H)
	if err != nil {
		return 0, 0, 0, err
	}
	gyRaw, err := m.readWord(MPU6050_GYRO_XOUT_H + 2)
	if err != nil {
		return 0, 0, 0, err
	}
	gzRaw, err := m.readWord(MPU6050_GYRO_XOUT_H + 4)
	if err != nil {
		return 0, 0, 0, err
	}

	x = float64(int16(gxRaw)) / 131.0
	y = float64(int16(gyRaw)) / 131.0
	z = float64(int16(gzRaw)) / 131.0
	return x, y, z, nil
}

func (m *MPU6050) ReadAll() (ax, ay, az, gx, gy, gz float64) {
	ax, ay, az = m.ReadAccel()
	time.Sleep(3 * time.Millisecond)

	gxRaw, gyRaw, gzRaw, _ := m.ReadGyroRaw()

	gx = gxRaw - m.gxOffset
	gy = gyRaw - m.gyOffset
	gz = gzRaw - m.gzOffset

	return
}

func (m *MPU6050) readByte(reg uint8) (uint8, error) {
	return m.bus.ReadRegU8(reg)
}

func (m *MPU6050) writeByte(reg uint8, val uint8) error {
	return m.bus.WriteRegU8(reg, val)
}

func (m *MPU6050) readWord(reg uint8) (uint16, error) {
	val, err := m.bus.ReadRegU16BE(reg)
	if err != nil {
		return 0, err
	}
	return val, nil
}
