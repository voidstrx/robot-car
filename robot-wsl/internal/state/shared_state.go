package state

import (
	"sync"
	"time"
)

type Mode string

const (
	ModeManual Mode = "MANUAL"
	ModeAuto   Mode = "AUTO"
)

// Telemetry — последние данные с робота
type Telemetry struct {
	Distance   float32
	Steering   float32
	Pan        float32
	Tilt       float32
	MotorA     float32
	MotorB     float32
	AccelX     float32
	AccelY     float32
	AccelZ     float32
	GyroX      float32
	GyroY      float32
	GyroZ      float32
	Timestamp  int64
	LastUpdate time.Time
}

// SharedState — общее состояние, доступное всем частям WSL
type SharedState struct {
	mu sync.RWMutex

	CurrentMode Mode
	Telemetry   Telemetry

	// Детекции (для Этапа 2)
	// DetectedObjects []DetectedObject
}

var Global = &SharedState{
	CurrentMode: ModeManual,
}

func (s *SharedState) SetMode(mode Mode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentMode = mode
}

func (s *SharedState) GetMode() Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CurrentMode
}

func (s *SharedState) UpdateTelemetry(t Telemetry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.LastUpdate = time.Now()
	s.Telemetry = t
}

func (s *SharedState) GetTelemetry() Telemetry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Telemetry
}
