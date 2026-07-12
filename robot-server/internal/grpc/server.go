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
	"robot-server/internal/webrtc"
)

type Server struct {
	pb.UnimplementedRobotControlServer
	servo         hardware.ServoController
	motor         hardware.MotorController
	ultra         hardware.Ultrasonic
	webrtcManager *webrtc.Manager
}

// === WebRTC Signaling ===
func (s *Server) WebRTCSignaling(stream pb.RobotControl_WebRTCSignalingServer) error {
	if s.webrtcManager == nil {
		return fmt.Errorf("webrtcManager is nil")
	}
	return s.webrtcManager.HandleSignaling(stream)
}

func NewServer(
	servo hardware.ServoController,
	motor hardware.MotorController,
	ultra hardware.Ultrasonic,
	webrtcManager *webrtc.Manager,
) *Server {
	return &Server{
		servo:         servo,
		motor:         motor,
		ultra:         ultra,
		webrtcManager: webrtcManager,
	}
}

// === Фильтр для сервоприводов (оставил как было) ===
var (
	smoothed = 0.0
	lastSent = 0.0
	isFirst  = true
)

func filter(raw float64) (float64, bool) {
	if isFirst {
		smoothed, lastSent, isFirst = raw, raw, false
		return raw, true
	}

	smoothed = 0.25*raw + 0.75*smoothed

	if math.Abs(smoothed-lastSent) >= 0.04 {
		lastSent = smoothed
		return lastSent, true
	}
	return lastSent, false
}

// === Основной стрим управления ===
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

		// Сервоприводы
		if s.servo != nil {
			s.servo.SetPosition(context.Background(), "steering", float64(cmd.Steering))
			s.servo.SetPosition(context.Background(), "pan", float64(cmd.Pan))
			s.servo.SetPosition(context.Background(), "tilt", float64(cmd.Tilt))
		}

		// Логика движения
		move := float64(cmd.Move)
		steer := float64(cmd.Steering)

		left := move
		right := move

		if move != 0 && steer != 0 {
			turnFactor := 0.65
			if steer > 0 {
				right -= math.Abs(steer) * turnFactor
			} else if steer < 0 {
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
