package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"os"
	"robot-client/input"
	"robot-client/internal/video"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "robot-client/internal/grpc/pb"
)

type Config struct {
	ServerAddr string `json:"server_addr"`
	RTSPURL    string `json:"rtsp_url"`
	Video      struct {
		OffsetY   float64 `json:"offset_y"`
		BarHeight int     `json:"bar_height"`
	} `json:"video"`
}

type Game struct {
	keyboard      *input.Keyboard
	gamepad       *input.Gamepad
	stream        pb.RobotControl_StreamControlClient
	cmdCh         chan *pb.Command
	lastCmd       *pb.Command
	connected     bool
	lastTelemetry *pb.Telemetry

	videoStream *video.RTSPStream

	videoOffsetY   float64
	videoBarHeight int
}

func loadConfig() Config {
	data, _ := os.ReadFile("configs/client.json")
	var cfg Config
	json.Unmarshal(data, &cfg)

	if cfg.ServerAddr == "" {
		cfg.ServerAddr = "192.168.137.225:50051"
	}
	if cfg.RTSPURL == "" {
		cfg.RTSPURL = "rtsp://192.168.137.225:8554/robot"
	}
	if cfg.Video.OffsetY == 0 {
		cfg.Video.OffsetY = -95
	}
	if cfg.Video.BarHeight == 0 {
		cfg.Video.BarHeight = 85
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
		log.Fatal(err)
	}
	client := pb.NewRobotControlClient(conn)

	stream, err := client.StreamControl(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	g.stream = stream
	g.connected = true

	go g.sendLoop()
	go g.recvLoop()

	g.videoStream = video.NewRTSPStream(cfg.RTSPURL)
	if err := g.videoStream.Start(); err != nil {
		log.Printf("RTSP error: %v", err)
	}

	g.videoOffsetY = cfg.Video.OffsetY
	g.videoBarHeight = cfg.Video.BarHeight

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
			g.connected = false
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

	cmd := &pb.Command{
		Steering: float32(g.gamepad.Steering() + g.keyboard.Steering()),
		Move:     float32(g.gamepad.Move() + g.keyboard.Move()),
		Pan:      float32(g.gamepad.Pan() + g.keyboard.Pan()),
		Tilt:     float32(g.gamepad.Tilt() + g.keyboard.Tilt()),
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
	// === Видео ===
	if g.videoStream != nil {
		frame := g.videoStream.GetFrame()
		if !frame.Empty() {
			img, _ := frame.ToImage()
			if img != nil {
				ebitenImg := ebiten.NewImageFromImage(img)
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(0, g.videoOffsetY)
				screen.DrawImage(ebitenImg, op)
			}
			frame.Close()
		}
	}

	// === Панель телеметрии ===
	barHeight := g.videoBarHeight
	barY := 720 - barHeight

	bar := ebiten.NewImage(1280, barHeight)
	bar.Fill(color.RGBA{0, 0, 0, 235})
	opBar := &ebiten.DrawImageOptions{}
	opBar.GeoM.Translate(0, float64(barY))
	screen.DrawImage(bar, opBar)

	y := barY + 8

	status := "OFFLINE"
	if g.connected {
		status = "CONNECTED"
	}

	if g.lastTelemetry != nil {
		t := g.lastTelemetry

		// Строка 1: Статус + Дистанция + Акселерометр
		ebitenutil.DebugPrintAt(screen,
			fmt.Sprintf("%s  |  Dist: %.1f m  |  Accel: X=%5.2f  Y=%5.2f  Z=%5.2f",
				status, t.Distance, t.AccelX, t.AccelY, t.AccelZ), 12, y)
		y += 20

		// Строка 2: Гироскоп + Моторы
		ebitenutil.DebugPrintAt(screen,
			fmt.Sprintf("Gyro:  X=%6.1f  Y=%6.1f  Z=%6.1f   |   MotorA: %.2f   MotorB: %.2f",
				t.GyroX, t.GyroY, t.GyroZ, t.MotorA, t.MotorB), 12, y)
		y += 20

		// Строка 3: Управление
		ebitenutil.DebugPrintAt(screen,
			fmt.Sprintf("Steer: %5.2f   Move: %5.2f   Pan: %5.2f   Tilt: %5.2f",
				g.lastCmd.Steering, g.lastCmd.Move, g.lastCmd.Pan, g.lastCmd.Tilt), 12, y)

	} else {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s  |  Ожидание телеметрии...", status), 12, y)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return 1280, 720
}

func main() {
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle("Robot Client")
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
