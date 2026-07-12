package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"robot-client/input"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "robot-client/internal/grpc/pb"
)

type Config struct {
	ServerAddr string `json:"server_addr"`
}

type Game struct {
	keyboard  *input.Keyboard // пока оставим, позже вынесем
	gamepad   *input.Gamepad
	stream    pb.RobotControl_StreamControlClient
	cmdCh     chan *pb.Command
	lastCmd   *pb.Command
	connected bool
}

func loadConfig() Config {
	data, _ := os.ReadFile("configs/client.json")
	var cfg Config
	json.Unmarshal(data, &cfg)
	if cfg.ServerAddr == "" {
		cfg.ServerAddr = "192.168.137.225:50051"
	}
	return cfg
}

func NewGame() *Game {
	cfg := loadConfig()
	g := &Game{
		keyboard: input.NewKeyboard(),
		gamepad:  input.NewGamepad(),
		cmdCh:    make(chan *pb.Command, 10),
		lastCmd:  &pb.Command{},
	}

	conn, err := grpc.Dial(cfg.ServerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("⚠️ Не удалось подключиться: %v", err)
	} else {
		client := pb.NewRobotControlClient(conn)
		g.stream, err = client.StreamControl(context.Background())
		if err != nil {
			log.Printf("Stream error: %v", err)
		} else {
			g.connected = true
			go g.sendLoop()
			go g.recvLoop()
			log.Println("✅ Подключено к", cfg.ServerAddr)
		}
	}
	return g
}

func (g *Game) sendLoop() {
	for cmd := range g.cmdCh {
		if g.stream != nil {
			g.stream.Send(cmd)
		}
	}
}

func (g *Game) recvLoop() {
	for {
		if g.stream == nil {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		tel, err := g.stream.Recv()
		if err != nil {
			return
		}
		fmt.Printf("\r[Tel] Dist:%.1f Steer:%.2f", tel.Distance, tel.Steering)
	}
}

func (g *Game) Update() error {
	g.keyboard.Update()
	g.gamepad.Update()

	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		os.Exit(0)
	}

	steer := g.gamepad.Steering()
	if steer == 0 {
		steer = g.keyboard.Steering()
	}
	move := g.gamepad.Move()
	if move == 0 {
		move = g.keyboard.Move()
	}
	pan := g.gamepad.Pan()
	if pan == 0 {
		pan = g.keyboard.Pan()
	}
	tilt := g.gamepad.Tilt()
	if tilt == 0 {
		tilt = g.keyboard.Tilt()
	}

	cmd := &pb.Command{
		Steering: float32(steer),
		Pan:      float32(pan),
		Tilt:     float32(tilt),
		Move:     float32(move),
	}
	g.lastCmd = cmd

	if g.connected {
		select {
		case g.cmdCh <- cmd:
		default:
		}
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	status := "OFFLINE"
	if g.connected {
		status = "CONNECTED"
	}
	ebitenutil.DebugPrint(screen, fmt.Sprintf(
		"Status: %s\n\nSteering: %.2f\nMove: %.2f\nPan: %.2f\nTilt: %.2f\n\nESC to quit",
		status, g.lastCmd.Steering, g.lastCmd.Move, g.lastCmd.Pan, g.lastCmd.Tilt))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return 640, 480
}

func main() {
	ebiten.SetWindowSize(640, 480)
	ebiten.SetWindowTitle("Robot Client")
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
