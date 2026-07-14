package grpcserver

import (
	"context"
	"io"
	"log"
	"math"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "robot-server/internal/grpc/pb"
	"robot-server/internal/hardware"
	"robot-server/internal/hardware/i2c"
)

type Server struct {
	pb.UnimplementedRobotControlServer
	servo      hardware.ServoController
	motor      hardware.MotorController
	ultra      hardware.Ultrasonic
	mpu        *i2c.MPU6050
	axisMap    map[string]string
	axisInvert map[string]bool
}

func NewServer(
	servo hardware.ServoController,
	motor hardware.MotorController,
	ultra hardware.Ultrasonic,
	mpu *i2c.MPU6050,
	axisMap map[string]string,
	axisInvert map[string]bool,
) *Server {
	if axisMap == nil {
		axisMap = make(map[string]string)
	}
	if axisInvert == nil {
		axisInvert = make(map[string]bool)
	}

	return &Server{
		servo:      servo,
		motor:      motor,
		ultra:      ultra,
		mpu:        mpu,
		axisMap:    axisMap,
		axisInvert: axisInvert,
	}
}

func (s *Server) StreamControl(stream pb.RobotControl_StreamControlServer) error {
	log.Println("gRPC клиент подключён")

	go func() {
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			ax, ay, az, gx, gy, gz := 0.0, 0.0, 0.0, 0.0, 0.0, 0.0

			if s.mpu != nil {
				ax, ay, az, gx, gy, gz = s.mpu.ReadAll()
			}

			distance := 0.0
			if s.ultra != nil {
				distance = s.ultra.GetDistance()
			}
			log.Printf("[Ultrasonic] Distance = %.1f", distance)
			tel := &pb.Telemetry{
				Distance:  float32(distance),
				Steering:  float32(s.servo.GetPosition("steering")),
				Pan:       float32(s.servo.GetPosition("pan")),
				Tilt:      float32(s.servo.GetPosition("tilt")),
				MotorA:    float32(s.motor.GetSpeed("a")),
				MotorB:    float32(s.motor.GetSpeed("b")),
				Timestamp: time.Now().UnixMilli(),
			}

			// Применяем маппинг + инверсию
			tel.AccelX = float32(applyAxis(ax, ay, az, "accel_x", s.axisMap, s.axisInvert))
			tel.AccelY = float32(applyAxis(ax, ay, az, "accel_y", s.axisMap, s.axisInvert))
			tel.AccelZ = float32(applyAxis(ax, ay, az, "accel_z", s.axisMap, s.axisInvert))

			tel.GyroX = float32(applyAxis(gx, gy, gz, "gyro_x", s.axisMap, s.axisInvert))
			tel.GyroY = float32(applyAxis(gx, gy, gz, "gyro_y", s.axisMap, s.axisInvert))
			tel.GyroZ = float32(applyAxis(gx, gy, gz, "gyro_z", s.axisMap, s.axisInvert))

			tel.MotorATarget = tel.MotorA
			tel.MotorBTarget = tel.MotorB

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

		if s.servo != nil {
			s.servo.SetPosition(context.Background(), "steering", float64(cmd.Steering))
			s.servo.SetPosition(context.Background(), "pan", float64(cmd.Pan))
			s.servo.SetPosition(context.Background(), "tilt", float64(cmd.Tilt))
		}

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
		return err
	}
	grpcServer := grpc.NewServer()
	pb.RegisterRobotControlServer(grpcServer, srv)
	reflection.Register(grpcServer)
	log.Printf("gRPC сервер запущен на %s", addr)
	return grpcServer.Serve(lis)
}

// applyAxis — применяет маппинг и инверсию осей
func applyAxis(rawX, rawY, rawZ float64, target string, mapping map[string]string, invert map[string]bool) float64 {
	source := mapping[target+"_source"]
	if source == "" {
		source = target // если маппинга нет — берём как есть
	}

	var value float64

	switch source {
	case "sensor_x", "accel_x", "gyro_x":
		value = rawX
	case "sensor_y", "accel_y", "gyro_y":
		value = rawY
	case "sensor_z", "accel_z", "gyro_z":
		value = rawZ
	default:
		value = rawX // fallback
	}

	if invert[target] {
		value = -value
	}

	return value
}
