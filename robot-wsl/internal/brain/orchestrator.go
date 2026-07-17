package brain

import (
	"robot-wsl/internal/input"
	"robot-wsl/internal/vision"
)

type Orchestrator struct {
	vision    *vision.Vision
	robot     *robot.Connection
	lastInput input.InputState
	// Можно добавить TaskGenerator, состояние режима и т.д.
}

func NewOrchestrator(vis *vision.Vision, robotConn *robot.Connection) *Orchestrator {
	return &Orchestrator{
		vision: vis,
		robot:  robotConn,
	}
}

// HandleInput вызывается, когда приходят данные от Windows
func (o *Orchestrator) HandleInput(state input.InputState) {
	o.lastInput = state
	// Здесь можно сразу реагировать на важные кнопки (например, переключение режима)
}

// Update вызывается каждый кадр
func (o *Orchestrator) Update() {
	// 1. Получаем детекции (Vision уже сделал рендеринг)
	detections := o.vision.GetLastDetections()

	// 2. Применяем логику поведения (TaskGenerator)
	// Например:
	// task := o.taskGenerator.Generate(detections, o.lastInput)
	// o.executeTask(task)

	// 3. Отправляем команды на робота (если нужно)
	// o.robot.SendCommand(...)
}
