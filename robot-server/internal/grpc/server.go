package grpc

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"robot-server/internal/hardware"
	"robot-server/internal/hardware/i2c"
	"robot-server/internal/state"
	pb "robot-server/internal/grpc/pb"
)

// Server реализует RobotControl
type Server struct {
	pb.UnimplementedRobotControlServer
	st *state.State

	// Hardware controllers (инициализируются в main.go)
	motorCtrl  hardware.MotorController
	servoCtrl  hardware.ServoController
	ultra      hardware.Ultrasonic
	ledCtrl    hardware.LEDController
	mpu        *i2c.MPU6050

	mu sync.Mutex
}

func NewServer(st *state.State, motor hardware.MotorController, servo hardware.ServoController, ultra hardware.Ultrasonic, led hardware.LEDController, mpu *i2c.MPU6050) *Server {
	return &Server{
		st:        st,
		motorCtrl: motor,
		servoCtrl: servo,
		ultra:     ultra,
		ledCtrl:   led,
		mpu:       mpu,
	}
}

// ====================== Режим ======================

func (s *Server) SetMode(ctx context.Context, req *pb.SetModeRequest) (*emptypb.Empty, error) {
	mode := state.Mode(req.Mode)
	s.st.SetMode(mode)
	log.Printf("[gRPC] SetMode → %v", mode)
	return &emptypb.Empty{}, nil
}

func (s *Server) GetMode(ctx context.Context, req *pb.GetModeRequest) (*pb.GetModeResponse, error) {
	mode := s.st.GetMode()
	return &pb.GetModeResponse{
		Mode: pb.Mode(mode),
	}, nil
}

// ====================== Команда управления (unary) ======================

func (s *Server) SendCommand(ctx context.Context, cmd *pb.ControlCommand) (*emptypb.Empty, error) {
	currentMode := s.st.GetMode()

	log.Printf("[gRPC] SendCommand (mode=%v): steering=%.2f move=%.2f pan=%.2f tilt=%.2f",
		currentMode, cmd.Steering, cmd.Move, cmd.Pan, cmd.Tilt)

	// В AUTO режиме команды от Windows игнорируем (управляет WSL)
	if currentMode == state.ModeAuto {
		return &emptypb.Empty{}, nil
	}

	// === Сервоприводы (steering, pan, tilt) ===
	if s.servoCtrl != nil {
		_ = s.servoCtrl.SetPosition(ctx, "steering", float64(cmd.Steering))
		_ = s.servoCtrl.SetPosition(ctx, "pan", float64(cmd.Pan))
		_ = s.servoCtrl.SetPosition(ctx, "tilt", float64(cmd.Tilt))
	}

	// === Моторы (дифференциальное управление) ===
	if s.motorCtrl != nil {
		move := float64(cmd.Move)
		steer := float64(cmd.Steering)

		left := move
		right := move

		if steer != 0 {
			turn := steer * 0.65
			if turn > 0 {
				right -= turn
			} else {
				left += turn
			}
		}

		_ = s.motorCtrl.SetSpeed(ctx, "a", left)
		_ = s.motorCtrl.SetSpeed(ctx, "b", right)
	}

	return &emptypb.Empty{}, nil
}

// ====================== Текстовое сообщение ======================

func (s *Server) SetMessage(ctx context.Context, req *pb.SetMessageRequest) (*emptypb.Empty, error) {
	s.st.SetMessage(req.Text)
	log.Printf("[gRPC] SetMessage: %q", req.Text)
	return &emptypb.Empty{}, nil
}

func (s *Server) HasPendingMessage(ctx context.Context, req *pb.HasPendingMessageRequest) (*pb.HasPendingMessageResponse, error) {
	has := s.st.HasPendingMessage()
	return &pb.HasPendingMessageResponse{
		HasMessage: has,
	}, nil
}

func (s *Server) TakeMessage(ctx context.Context, req *pb.TakeMessageRequest) (*pb.TakeMessageResponse, error) {
	text, ok := s.st.TakeMessage()
	if !ok {
		return &pb.TakeMessageResponse{Text: ""}, nil
	}
	log.Printf("[gRPC] TakeMessage: %q", text)
	return &pb.TakeMessageResponse{
		Text: text,
	}, nil
}

// ====================== Постоянный поток телеметрии (Pi → WSL) ======================

func (s *Server) StreamTelemetry(req *emptypb.Empty, stream pb.RobotControl_StreamTelemetryServer) error {
	log.Println("[gRPC] StreamTelemetry: клиент подключился")

	ticker := time.NewTicker(100 * time.Millisecond) // 10 Гц
	defer ticker.Stop()

	for {
		select {
		case <-stream.Context().Done():
			log.Println("[gRPC] StreamTelemetry: клиент отключился")
			return nil
		case <-ticker.C:
			tel := &pb.Telemetry{
				Timestamp: time.Now().UnixMilli(),
			}

			// === Данные с hardware ===
			if s.ultra != nil {
				tel.Distance = float32(s.ultra.GetDistance())
			}

			if s.motorCtrl != nil {
				tel.MotorA = float32(s.motorCtrl.GetSpeed("a"))
				tel.MotorB = float32(s.motorCtrl.GetSpeed("b"))
			}

			if s.servoCtrl != nil {
				tel.Steering = float32(s.servoCtrl.GetPosition("steering"))
				tel.Pan = float32(s.servoCtrl.GetPosition("pan"))
				tel.Tilt = float32(s.servoCtrl.GetPosition("tilt"))
			}

			if s.mpu != nil {
				ax, ay, az, gx, gy, gz := s.mpu.ReadAll()
				tel.AccelX = float32(ax)
				tel.AccelY = float32(ay)
				tel.AccelZ = float32(az)
				tel.GyroX = float32(gx)
				tel.GyroY = float32(gy)
				tel.GyroZ = float32(gz)
			}

			if err := stream.Send(tel); err != nil {
				log.Printf("[gRPC] StreamTelemetry send error: %v", err)
				return err
			}
		}
	}
}

// ====================== Запуск сервера ======================

func StartGRPCServer(addr string, srv *Server) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	grpcServer := grpc.NewServer()
	pb.RegisterRobotControlServer(grpcServer, srv)

	log.Printf("[gRPC] Listening on %s", addr)
	return grpcServer.Serve(lis)
}
