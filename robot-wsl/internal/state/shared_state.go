package state

import (
	"log"
	"sync"
	"time"
)

// Режимы работы
type Mode string

const (
	ModeManual     Mode = "MANUAL"
	ModeHybrid     Mode = "HYBRID"
	ModeAutonomous Mode = "AUTONOMOUS"
)

// Command — команда, которую отправляет мозг или человек
type Command struct {
	Steering float32
	Move     float32
	Pan      float32
	Tilt     float32
}

// DetectedObject — результат работы компьютерного зрения
type DetectedObject struct {
	Class      string // "cat", "person", "car" и т.д.
	TrackID    string // ID для трекинга (если используется)
	Confidence float32
	X          float32 // нормализованная координата центра по X (-1.0 .. +1.0)
	Y          float32 // нормализованная координата центра по Y (-1.0 .. +1.0)
	Width      float32 // нормализованная ширина
	Height     float32 // нормализованная высота
	Timestamp  time.Time
}

// SharedState — общее состояние, доступное и GUI, и мозгу
type SharedState struct {
	mu sync.RWMutex

	CurrentMode Mode
	LastUpdate  time.Time

	// === Телеметрия с робота ===
	Distance float32
	Accel    [3]float32
	Gyro     [3]float32
	MotorA   float32
	MotorB   float32

	// === Результаты компьютерного зрения ===
	DetectedObjects []DetectedObject
	LastCat         *DetectedObject // для удобного доступа к последнему обнаруженному коту

	// === Команды ===
	LastBrainCommand *Command
	UserOverride     bool // true, если сейчас управляет человек
}

// Глобальное состояние (можно использовать напрямую)
var GlobalState = &SharedState{
	CurrentMode: ModeHybrid,
}

// ================== Методы ==================

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

// CycleMode — переключает режимы по кругу: Manual → Hybrid → Autonomous
func (s *SharedState) CycleMode() {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.CurrentMode {
	case ModeManual:
		s.CurrentMode = ModeHybrid
	case ModeHybrid:
		s.CurrentMode = ModeAutonomous
	case ModeAutonomous:
		s.CurrentMode = ModeManual
	default:
		s.CurrentMode = ModeHybrid
	}

	log.Printf("[State] Режим переключён на: %s", s.CurrentMode)
}

func (s *SharedState) SetUserOverride(active bool) {
	s.mu.Lock()
	s.UserOverride = active
	s.mu.Unlock()
}

func (s *SharedState) UpdateTelemetry(distance float32, accel, gyro [3]float32, motorA, motorB float32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Distance = distance
	s.Accel = accel
	s.Gyro = gyro
	s.MotorA = motorA
	s.MotorB = motorB
	s.LastUpdate = time.Now()
}

// UpdateDetectedObjects — обновляет результаты детекции от Vision
func (s *SharedState) UpdateDetectedObjects(objects []DetectedObject) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.DetectedObjects = objects
	s.LastUpdate = time.Now()

	// Ищем кота среди обнаруженных объектов
	s.LastCat = nil
	for i := range objects {
		if objects[i].Class == "cat" {
			s.LastCat = &objects[i]
			break
		}
	}
}

func (s *SharedState) GetDetectedObjects() []DetectedObject {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.DetectedObjects
}

func (s *SharedState) GetLastCat() *DetectedObject {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastCat
}
