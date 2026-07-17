package state

import (
	"sync"

	"robot-server/internal/hardware"
)

// robotState — потокобезопасная реализация hardware.RobotState
type robotState struct {
	mu sync.RWMutex

	distance float64
	servos   map[string]float64
	motors   map[string]float64
	leds     map[string]bool
}

// NewRobotState создаёт новое состояние робота
func NewRobotState() hardware.RobotState {
	return &robotState{
		servos: make(map[string]float64),
		motors: make(map[string]float64),
		leds:   make(map[string]bool),
	}
}

// --- Getters ---

func (s *robotState) GetDistance() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.distance
}

func (s *robotState) GetServoPosition(name string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.servos[name]
}

func (s *robotState) GetMotorSpeed(motor string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.motors[motor]
}

func (s *robotState) GetSimpleLEDState(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.leds[id]
}

// --- Updaters ---

func (s *robotState) UpdateDistance(d float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.distance = d
}

func (s *robotState) UpdateServoPosition(name string, pos float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.servos[name] = pos
}

func (s *robotState) UpdateMotorSpeed(motor string, speed float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.motors[motor] = speed
}

func (s *robotState) UpdateSimpleLED(id string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leds[id] = on
}
