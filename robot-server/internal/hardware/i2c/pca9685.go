package i2c

import (
	"context"
	"fmt"
	"time"

	"github.com/d2r2/go-i2c"
	"github.com/d2r2/go-logger"

	"robot-server/internal/hardware"
)

const (
	PCA9685_MODE1     = 0x00
	PCA9685_PRESCALE  = 0xFE
	PCA9685_LED0_ON_L = 0x06
)

type ServoConfig struct {
	Channel     int  `json:"channel"`
	MinPulse    int  `json:"min_pulse"`
	CenterPulse int  `json:"center_pulse"`
	MaxPulse    int  `json:"max_pulse"`
	Invert      bool `json:"invert"`
}

type pca9685Controller struct {
	bus     *i2c.I2C
	state   hardware.RobotState
	configs map[string]ServoConfig
}

func NewPCA9685ServoController(busNum, addr int, servoConfigs map[string]ServoConfig, state hardware.RobotState) (hardware.ServoController, error) {
	// Отключаем debug-логи i2c
	logger.ChangePackageLogLevel("i2c", logger.ErrorLevel)

	bus, err := i2c.NewI2C(uint8(addr), busNum)
	if err != nil {
		return nil, fmt.Errorf("i2c.NewI2C: %w", err)
	}

	// Инициализация PCA9685 (50Hz для серв)
	if err := bus.WriteRegU8(PCA9685_MODE1, 0x00); err != nil {
		bus.Close()
		return nil, err
	}
	if err := bus.WriteRegU8(PCA9685_PRESCALE, 0x79); err != nil { // ~50-60Hz
		bus.Close()
		return nil, err
	}
	time.Sleep(10 * time.Millisecond)

	// Wake up
	if err := bus.WriteRegU8(PCA9685_MODE1, 0x20); err != nil {
		bus.Close()
		return nil, err
	}

	ctrl := &pca9685Controller{
		bus:     bus,
		state:   state,
		configs: servoConfigs,
	}

	// Инициализируем позиции в state
	for name := range servoConfigs {
		ctrl.state.UpdateServoPosition(name, 0)
	}

	fmt.Println("✅ PCA9685 инициализирован (адрес 0x40)")
	return ctrl, nil
}

func (c *pca9685Controller) calculatePulse(norm float64, cfg ServoConfig) int {
	if norm < -1.0 {
		norm = -1.0
	}
	if norm > 1.0 {
		norm = 1.0
	}
	if cfg.Invert {
		norm = -norm
	}

	var pulse int
	if norm <= 0 {
		pulse = cfg.CenterPulse + int(norm*float64(cfg.CenterPulse-cfg.MinPulse))
	} else {
		pulse = cfg.CenterPulse + int(norm*float64(cfg.MaxPulse-cfg.CenterPulse))
	}

	if pulse < cfg.MinPulse {
		pulse = cfg.MinPulse
	}
	if pulse > cfg.MaxPulse {
		pulse = cfg.MaxPulse
	}
	return pulse
}

func (c *pca9685Controller) writePulse(channel int, pulse int) error {
	reg := uint8(PCA9685_LED0_ON_L + channel*4)
	if err := c.bus.WriteRegU8(reg+0, 0); err != nil {
		return err
	}
	if err := c.bus.WriteRegU8(reg+1, 0); err != nil {
		return err
	}
	if err := c.bus.WriteRegU8(reg+2, byte(pulse&0xFF)); err != nil {
		return err
	}
	return c.bus.WriteRegU8(reg+3, byte(pulse>>8))
}

func (c *pca9685Controller) SetPosition(ctx context.Context, name string, position float64) error {
	cfg, ok := c.configs[name]
	if !ok {
		return fmt.Errorf("servo %s not configured", name)
	}

	pulse := c.calculatePulse(position, cfg)
	if err := c.writePulse(cfg.Channel, pulse); err != nil {
		return err
	}

	c.state.UpdateServoPosition(name, position)
	fmt.Printf("🔧 Servo %s: %.2f → pulse %d\n", name, position, pulse)
	return nil
}

func (c *pca9685Controller) GetPosition(name string) float64 {
	return c.state.GetServoPosition(name)
}

func (c *pca9685Controller) Release(name string) error {
	cfg, ok := c.configs[name]
	if !ok {
		return fmt.Errorf("servo %s not found", name)
	}
	reg := uint8(PCA9685_LED0_ON_L + cfg.Channel*4)
	c.bus.WriteRegU8(reg+0, 0)
	c.bus.WriteRegU8(reg+1, 0)
	c.bus.WriteRegU8(reg+2, 0)
	c.bus.WriteRegU8(reg+3, 0)
	fmt.Printf("🔓 Servo %s released\n", name)
	return nil
}

func (c *pca9685Controller) ReleaseAll() error {
	for name := range c.configs {
		c.Release(name)
	}
	return nil
}

func (c *pca9685Controller) Close() error {
	c.ReleaseAll()
	c.bus.Close()
	fmt.Println("🔌 PCA9685 закрыт")
	return nil
}
