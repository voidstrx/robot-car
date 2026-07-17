package execution

import (
	"log"
	"sync"

	pb "robot-client/internal/grpc/pb" // твой сгенерированный protobuf
	"robot-client/internal/state"
)

type Executor struct {
	stream  pb.RobotControl_StreamControlClient
	state   *state.SharedState
	mu      sync.Mutex
	enabled bool
}

func NewExecutor(stream pb.RobotControl_StreamControlClient, st *state.SharedState) *Executor {
	return &Executor{
		stream:  stream,
		state:   st,
		enabled: true,
	}
}

// SendCommand отправляет команду на робота
func (e *Executor) SendCommand(cmd *state.Command) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled {
		return nil
	}

	// Проверка на ручное вмешательство в Hybrid режиме
	if e.state.GetMode() == state.ModeHybrid && e.state.UserOverride {
		log.Println("[Execution] Пропуск команды мозга (пользователь управляет)")
		return nil
	}

	pbCmd := &pb.Command{
		Steering: float32(cmd.Steering),
		Move:     float32(cmd.Move),
		Pan:      float32(cmd.Pan),
		Tilt:     float32(cmd.Tilt),
	}

	if err := e.stream.Send(pbCmd); err != nil {
		log.Printf("[Execution] Ошибка отправки команды: %v", err)
		return err
	}

	// Сохраняем последнюю отправленную команду
	e.state.LastBrainCommand = cmd
	//log.Printf("[Execution] Отправлена команда: steer=%.2f, move=%.2f", cmd.Steering, cmd.Move)

	return nil
}

// Enable / Disable можно использовать для временной приостановки мозга
func (e *Executor) Enable() {
	e.mu.Lock()
	e.enabled = true
	e.mu.Unlock()
}

func (e *Executor) Disable() {
	e.mu.Lock()
	e.enabled = false
	e.mu.Unlock()
}
