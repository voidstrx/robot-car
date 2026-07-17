package planning

import (
	"robot-client/internal/state"
)

type TaskGenerator interface {
	GenerateTask(st *state.SharedState) *state.Command
}

// SimpleTaskGenerator — простая логика на правилах
type SimpleTaskGenerator struct{}

func NewSimpleTaskGenerator() *SimpleTaskGenerator {
	return &SimpleTaskGenerator{}
}

func (g *SimpleTaskGenerator) GenerateTask(st *state.SharedState) *state.Command {
	distance := st.Distance

	// Если очень близко препятствие — поворачиваем и немного едем назад
	if distance > 0 && distance < 0.35 {
		return &state.Command{
			Steering: 0.0,
			Move:     0.0,
			Pan:      0.0,
			Tilt:     0.5,
		}
	}

	// Обычное движение вперёд
	return &state.Command{
		Steering: 0.0,
		Move:     0.0,
		Pan:      0.0,
		Tilt:     -0.2,
	}
}
