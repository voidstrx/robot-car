package grpcserver

import (
	"context"
	"fmt"
	"io"
	"log"
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

		// Применяем к железу
		if s.servo != nil {
			s.servo.SetPosition(context.Background(), "steering", float64(cmd.Steering))
			s.servo.SetPosition(context.Background(), "pan", float64(cmd.Pan))
			s.servo.SetPosition(context.Background(), "tilt", float64(cmd.Tilt))
		}

		base := float64(cmd.Move)
		left := base
		right := base
		if cmd.Steering > 0 {
			right -= float64(cmd.Steering) * 0.55
		} else if cmd.Steering < 0 {
			left += float64(cmd.Steering) * 0.55
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
