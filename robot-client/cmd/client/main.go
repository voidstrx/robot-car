package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"robot-client/input"
	"robot-client/internal/webrtc"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "robot-client/internal/grpc/pb"
)

type Config struct {
	ServerAddr string `json:"server_addr"`
	WebRTC     struct {
		Enabled bool `json:"enabled"`
	} `json:"webrtc"`
}

type Game struct {
	keyboard      *input.Keyboard
	gamepad       *input.Gamepad
	stream        pb.RobotControl_StreamControlClient
	cmdCh         chan *pb.Command
	lastCmd       *pb.Command
	connected     bool
	webrtcClient  *webrtc.Client
	lastTelemetry *pb.Telemetry
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

func (g *Game) connectWithRetry(addr string) (*grpc.ClientConn, error) {
	for attempt := 0; attempt < 30; attempt++ {
		conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			return conn, nil
		}
		time.Sleep(time.Duration(500*(attempt+1)) * time.Millisecond)
	}
	return nil, fmt.Errorf("не удалось подключиться")
}

func NewGame() *Game {
	cfg := loadConfig()
	g := &Game{
		keyboard: input.NewKeyboard(),
		gamepad:  input.NewGamepad(),
		cmdCh:    make(chan *pb.Command, 10),
		lastCmd:  &pb.Command{},
	}

	conn, err := g.connectWithRetry(cfg.ServerAddr)
	if err != nil {
		log.Fatal(err)
	}
	client := pb.NewRobotControlClient(conn)

	controlStream, _ := client.StreamControl(context.Background())
	g.stream = controlStream
	g.connected = true

	webrtcStream, _ := client.WebRTCSignaling(context.Background())
	g.webrtcClient = webrtc.NewClient()

	go g.sendLoop()
	go g.recvLoop()

	if cfg.WebRTC.Enabled {
		g.webrtcClient.Start(webrtcStream)
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
		g.lastTelemetry = tel
	}
}

func (g *Game) Update() error {
	g.keyboard.Update()
	g.gamepad.Update()
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		os.Exit(0)
	}

	// ... (логика управления без изменений) ...
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

	cmd := &pb.Command{Steering: float32(steer), Pan: float32(pan), Tilt: float32(tilt), Move: float32(move)}
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
	g.webrtcClient.Draw(screen)

	status := "OFFLINE"
	if g.connected {
		status = "CONNECTED"
	}
	webrtcStatus, videoStatus := g.webrtcClient.GetStatuses()

	y := 10
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("gRPC:   %s", status), 10, y)
	y += 16
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("WebRTC: %s", webrtcStatus), 10, y)
	y += 16
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Video:  %s", videoStatus), 10, y)
	y += 16
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Steering: %.2f  Move: %.2f", g.lastCmd.Steering, g.lastCmd.Move), 10, y)
	y += 16

	if g.lastTelemetry != nil {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Distance: %.1f", g.lastTelemetry.Distance), 10, y)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return 640, 480
}

func main() {
	log.SetOutput(os.Stdout)
	ebiten.SetWindowSize(640, 480)
	ebiten.SetWindowTitle("Robot Client")
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
