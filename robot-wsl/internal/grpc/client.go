package grpc

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	"robot-wsl/internal/state"
	pb "robot-wsl/internal/grpc/pb"
)

// Client — gRPC клиент к robot-server (Raspberry Pi)
type Client struct {
	addr   string
	conn   *grpc.ClientConn
	client pb.RobotControlClient
}

func NewClient(addr string) *Client {
	return &Client{addr: addr}
}

func (c *Client) Connect() error {
	conn, err := grpc.NewClient(c.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	c.conn = conn
	c.client = pb.NewRobotControlClient(conn)
	log.Printf("[gRPC] Connected to %s", c.addr)
	return nil
}

// StartModePolling — каждые 100 мс запрашивает режим и пишет в SharedState
func (c *Client) StartModePolling(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				resp, err := c.client.GetMode(ctx, &pb.GetModeRequest{})
				if err != nil {
					log.Printf("[gRPC] GetMode error: %v", err)
					continue
				}

				mode := state.ModeManual
				if resp.Mode == pb.Mode_MODE_AUTO {
					mode = state.ModeAuto
				}
				state.Global.SetMode(mode)
			}
		}
	}()
}

// StartTelemetryStream — постоянно получает телеметрию
func (c *Client) StartTelemetryStream(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			stream, err := c.client.StreamTelemetry(ctx, &emptypb.Empty{})
			if err != nil {
				log.Printf("[gRPC] StreamTelemetry connect error: %v", err)
				time.Sleep(2 * time.Second)
				continue
			}

			log.Println("[gRPC] StreamTelemetry connected")

			for {
				tel, err := stream.Recv()
				if err != nil {
					log.Printf("[gRPC] StreamTelemetry recv error: %v", err)
					break
				}

				state.Global.UpdateTelemetry(state.Telemetry{
					Distance:  tel.Distance,
					Steering:  tel.Steering,
					Pan:       tel.Pan,
					Tilt:      tel.Tilt,
					MotorA:    tel.MotorA,
					MotorB:    tel.MotorB,
					AccelX:    tel.AccelX,
					AccelY:    tel.AccelY,
					AccelZ:    tel.AccelZ,
					GyroX:     tel.GyroX,
					GyroY:     tel.GyroY,
					GyroZ:     tel.GyroZ,
					Timestamp: tel.Timestamp,
				})
			}

			time.Sleep(1 * time.Second)
		}
	}()
}
