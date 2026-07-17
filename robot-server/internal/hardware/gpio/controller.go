package gpio

import "github.com/stianeikeland/go-rpio/v4"

// InitGPIO инициализирует GPIO (нужно вызывать один раз в main)
func InitGPIO() error {
	return rpio.Open()
}

// CloseGPIO закрывает GPIO
func CloseGPIO() {
	rpio.Close()
}
