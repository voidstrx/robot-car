package grpc

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "robot-client/internal/grpc/pb"
)

// Client — прямой gRPC-клиент к Raspberry Pi
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
	log.Printf("[gRPC Client] Connected to %s", c.addr)
	return nil
}

// SetMode переключает режим на Raspberry Pi
func (c *Client) SetMode(mode pb.Mode) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := c.client.SetMode(ctx, &pb.SetModeRequest{Mode: mode})
	if err != nil {
		return err
	}
	log.Printf("[gRPC] SetMode → %v", mode)
	return nil
}

// SendCommand отправляет команду управления (используется только в MANUAL)
func (c *Client) SendCommand(steering, move, pan, tilt float32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := c.client.SendCommand(ctx, &pb.ControlCommand{
		Steering: steering,
		Move:     move,
		Pan:      pan,
		Tilt:     tilt,
	})
	return err
}

// SetMessage отправляет текстовую задачу для Orchestrator
func (c *Client) SetMessage(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := c.client.SetMessage(ctx, &pb.SetMessageRequest{Text: text})
	if err != nil {
		return err
	}
	log.Printf("[gRPC] SetMessage: %q", text)
	return nil
}
