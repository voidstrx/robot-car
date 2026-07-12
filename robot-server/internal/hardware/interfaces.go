package hardware

import "context"

// Ultrasonic — интерфейс для HC-SR04
type Ultrasonic interface {
	Start(ctx context.Context, intervalMs int) error
	Stop()
	GetDistance() float64
}

// ServoController — PCA9685, работаем с именами серв (steering, pan, tilt) и нормализованными значениями -1.0..+1.0
type ServoController interface {
	SetPosition(ctx context.Context, name string, position float64) error
	GetPosition(name string) float64
	Release(name string) error
	ReleaseAll() error
	Close() error
}

// MotorController — два мотора (a и b), скорость -1.0..+1.0
type MotorController interface {
	SetSpeed(ctx context.Context, motor string, speed float64) error
	Stop(motor string) error
	StopAll() error
	GetSpeed(motor string) float64
	Close() error
}

// LEDController — простые GPIO + WS2811
type LEDController interface {
	SetSimpleLED(id string, on bool) error
	SetAllSimpleLEDs(on bool) error
	SetWS2811Pixel(index int, r, g, b byte) error
	SetAllWS2811(r, g, b byte) error
	RenderWS2811() error
	Close() error
}

// RobotState — потокобезопасное состояние
type RobotState interface {
	GetDistance() float64
	GetServoPosition(name string) float64
	GetMotorSpeed(motor string) float64
	GetSimpleLEDState(id string) bool

	UpdateDistance(d float64)
	UpdateServoPosition(name string, pos float64)
	UpdateMotorSpeed(motor string, speed float64)
	UpdateSimpleLED(id string, on bool)
}
