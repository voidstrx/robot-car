package grpcserver

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "robot-server/internal/grpc/pb"
	"robot-server/internal/hardware"
)

type Server struct {
	pb.UnimplementedRobotControlServer
	servo hardware.ServoController
	motor hardware.MotorController
	ultra hardware.Ultrasonic
}

func NewServer(servo hardware.ServoController, motor hardware.MotorController, ultra hardware.Ultrasonic) *Server {
	return &Server{servo: servo, motor: motor, ultra: ultra}
}

var (
	smoothed = 0.0  // Текущее сглаженное значение
	lastSent = 0.0  // Последнее отправленное значение
	isFirst  = true // Флаг первого запуска
)

func filter(raw float64) (float64, bool) {
	if isFirst {
		smoothed, lastSent, isFirst = raw, raw, false
		return raw, true
	}

	// 1. Сглаживание (EMA)
	smoothed = 0.25*raw + 0.75*smoothed

	// 2. Мертвая зона (Deadzone). Порог 0.04 (около 2% от диапазона)
	if math.Abs(smoothed-lastSent) >= 0.04 {
		lastSent = smoothed
		return lastSent, true // Значение изменилось, нужно отправить на серву
	}
	return lastSent, false // Изменение слишком мало, игнорируем
}

func (s *Server) StreamControl(stream pb.RobotControl_StreamControlServer) error {
	log.Println("🔌 gRPC клиент подключён")

	// Горутина отправки телеметрии
	go func() {
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			tel := &pb.Telemetry{
				Distance:  float32(s.ultra.GetDistance()),
				Steering:  float32(s.servo.GetPosition("steering")),
				Pan:       float32(s.servo.GetPosition("pan")),
				Tilt:      float32(s.servo.GetPosition("tilt")),
				MotorA:    float32(s.motor.GetSpeed("a")),
				MotorB:    float32(s.motor.GetSpeed("b")),
				Timestamp: time.Now().UnixMilli(),
			}
			if err := stream.Send(tel); err != nil {
				return
			}
		}
	}()

	for {
		cmd, err := stream.Recv()
		if err == io.EOF {
			log.Println("gRPC клиент отключился")
			return nil
		}
		if err != nil {
			return err
		}

		// === ВЫВОД ПОЛУЧЕННОЙ КОМАНДЫ ===
		log.Printf("📥 Получена команда: steer=%.2f move=%.2f pan=%.2f tilt=%.2f",
			cmd.Steering, cmd.Move, cmd.Pan, cmd.Tilt)

		// Применяем сервоприводы
		if s.servo != nil {
			s.servo.SetPosition(context.Background(), "steering", float64(cmd.Steering))
			s.servo.SetPosition(context.Background(), "pan", float64(cmd.Pan))
			s.servo.SetPosition(context.Background(), "tilt", float64(cmd.Tilt))
		}

		// === ЛОГИКА ДВИЖЕНИЯ: поворот только при Move ≠ 0 ===
		move := float64(cmd.Move)
		steer := float64(cmd.Steering)

		left := move
		right := move

		if move != 0 && steer != 0 {
			turnFactor := 0.65

			if steer > 0 { // поворот вправо
				right -= math.Abs(steer) * turnFactor
			} else if steer < 0 { // поворот влево
				left -= math.Abs(steer) * turnFactor
			}
		}

		s.motor.SetSpeed(context.Background(), "a", left)
		s.motor.SetSpeed(context.Background(), "b", right)
	}
}

func StartGRPCServer(addr string, srv *Server) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterRobotControlServer(grpcServer, srv)
	reflection.Register(grpcServer)

	log.Printf("✅ gRPC сервер запущен на %s", addr)
	return grpcServer.Serve(lis)
}
