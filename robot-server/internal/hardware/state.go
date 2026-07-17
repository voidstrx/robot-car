package hardware

import "sync"

// State реализует RobotState с защитой от race condition
type State struct {
	mu            sync.RWMutex
	distance      float64
	servoPos      map[string]float64
	motorSpeed    map[string]float64
	simpleLEDs    map[string]bool
}

func NewState() *State {
	return &State{
		servoPos:   make(map[string]float64),
		motorSpeed: make(map[string]float64),
		simpleLEDs: make(map[string]bool),
	}
}

func (s *State) GetDistance() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.distance
}

func (s *State) UpdateDistance(d float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.distance = d
}

func (s *State) GetServoPosition(name string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if pos, ok := s.servoPos[name]; ok {
		return pos
	}
	return 0
}

func (s *State) UpdateServoPosition(name string, pos float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pos < -1 { pos = -1 }
	if pos > 1 { pos = 1 }
	s.servoPos[name] = pos
}

func (s *State) GetMotorSpeed(motor string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sp, ok := s.motorSpeed[motor]; ok {
		return sp
	}
	return 0
}

func (s *State) UpdateMotorSpeed(motor string, speed float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if speed < -1 { speed = -1 }
	if speed > 1 { speed = 1 }
	s.motorSpeed[motor] = speed
}

func (s *State) GetSimpleLEDState(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if on, ok := s.simpleLEDs[id]; ok {
		return on
	}
	return false
}

func (s *State) UpdateSimpleLED(id string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.simpleLEDs[id] = on
}
