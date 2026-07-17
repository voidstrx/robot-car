package state

import (
	"sync"
)

// Mode соответствует enum из protobuf
type Mode int

const (
	ModeUnspecified Mode = 0
	ModeManual      Mode = 1
	ModeAuto        Mode = 2
)

// State — потокобезопасное состояние сервера на Raspberry Pi
type State struct {
	mu sync.RWMutex

	currentMode Mode

	// Одно текстовое сообщение (задача для Orchestrator)
	pendingMessage     string
	hasPendingMessage  bool
}

func New() *State {
	return &State{
		currentMode: ModeManual, // по умолчанию Manual
	}
}

// ---------- Режим ----------

func (s *State) SetMode(m Mode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentMode = m
}

func (s *State) GetMode() Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentMode
}

// ---------- Текстовое сообщение ----------

func (s *State) SetMessage(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingMessage = text
	s.hasPendingMessage = true
}

func (s *State) HasPendingMessage() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasPendingMessage
}

// TakeMessage возвращает сообщение и очищает его
func (s *State) TakeMessage() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.hasPendingMessage {
		return "", false
	}

	msg := s.pendingMessage
	s.pendingMessage = ""
	s.hasPendingMessage = false
	return msg, true
}
