package client

import (
	"fmt"
	"image/color"
	"log"
	"robot-client/input"
	"robot-client/internal/video"
	"time"

	"robot-client/internal/config"
	"robot-client/internal/state"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	pb "robot-client/internal/grpc/pb"
)

type Game struct {
	state         *state.SharedState
	cfg           *config.Config
	lastToggleKey bool

	// === Оригинальные поля ===
	keyboard      *input.Keyboard
	gamepad       *input.Gamepad
	stream        pb.RobotControl_StreamControlClient
	cmdCh         chan *pb.Command
	lastCmd       *pb.Command
	connected     bool
	lastTelemetry *pb.Telemetry

	videoStream    *video.RTSPStream
	videoOffsetY   float64
	videoBarHeight int
}

func NewGame(st *state.SharedState, cfg *config.Config, stream pb.RobotControl_StreamControlClient) *Game {
	g := &Game{
		state:     st,
		cfg:       cfg,
		keyboard:  input.NewKeyboard(),
		gamepad:   input.NewGamepad(),
		cmdCh:     make(chan *pb.Command, 10),
		lastCmd:   &pb.Command{},
		stream:    stream,
		connected: true,
	}

	// Запускаем горутины отправки и получения
	go g.sendLoop()
	go g.recvLoop()

	// Видео
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

	// === Переключение режимов ===
	toggleKey := ebiten.KeyF
	if g.cfg.Controls.ToggleAutonomousKey == "a" {
		toggleKey = ebiten.KeyA
	}

	if ebiten.IsKeyPressed(toggleKey) && !g.lastToggleKey {
		g.state.CycleMode()
	}
	g.lastToggleKey = ebiten.IsKeyPressed(toggleKey)

	// === Ручное управление ===
	cmd := &pb.Command{
		Steering: float32(g.gamepad.Steering() + g.keyboard.Steering()),
		Move:     float32(g.gamepad.Move() + g.keyboard.Move()),
		Pan:      float32(g.gamepad.Pan() + g.keyboard.Pan()),
		Tilt:     float32(g.gamepad.Tilt() + g.keyboard.Tilt()),
	}
	g.lastCmd = cmd

	if g.connected && g.state.GetMode() != state.ModeAutonomous {
		select {
		case g.cmdCh <- cmd:
		default:
		}
	}

	// UserOverride
	if cmd.Steering != 0 || cmd.Move != 0 {
		g.state.SetUserOverride(true)
	} else {
		g.state.SetUserOverride(false)
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

	// === Отрисовка детекций ===
	detections := g.state.GetDetectedObjects()

	for _, det := range detections {
		// Переводим координаты из [-1..1] в экранные
		x := (det.X + 1) / 2 * float32(g.cfg.Window.Width)
		y := (det.Y + 1) / 2 * float32(g.cfg.Window.Height)
		w := det.Width * float32(g.cfg.Window.Width)
		h := det.Height * float32(g.cfg.Window.Height)

		// Рисуем зелёную рамку
		vector.StrokeRect(screen, x-w/2, y-h/2, w, h, 2, color.RGBA{0, 255, 0, 255}, false)

		// Подпись
		label := fmt.Sprintf("%s %.0f%%", det.Class, det.Confidence*100)
		ebitenutil.DebugPrintAt(screen, label, int(x-w/2), int(y-h/2-18))
	}

	// === Панель телеметрии (оригинал) ===
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

	// === Показываем текущий режим ===
	modeText := "Mode: " + string(g.state.GetMode())
	if g.state.GetMode() == state.ModeHybrid && g.state.UserOverride {
		modeText += " (РУЧНОЕ)"
	}
	ebitenutil.DebugPrintAt(screen, modeText, 12, y)
	y += 20

	if g.lastTelemetry != nil {
		t := g.lastTelemetry

		ebitenutil.DebugPrintAt(screen,
			fmt.Sprintf("%s  |  Dist: %.1f m  |  Accel: X=%5.2f  Y=%5.2f  Z=%5.2f",
				status, t.Distance, t.AccelX, t.AccelY, t.AccelZ), 12, y)
		y += 20

		ebitenutil.DebugPrintAt(screen,
			fmt.Sprintf("Gyro:  X=%6.1f  Y=%6.1f  Z=%6.1f   |   MotorA: %.2f   MotorB: %.2f",
				t.GyroX, t.GyroY, t.GyroZ, t.MotorA, t.MotorB), 12, y)
		y += 20

		ebitenutil.DebugPrintAt(screen,
			fmt.Sprintf("Steer: %5.2f   Move: %5.2f   Pan: %5.2f   Tilt: %5.2f",
				g.lastCmd.Steering, g.lastCmd.Move, g.lastCmd.Pan, g.lastCmd.Tilt), 12, y)
	} else {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s  |  Ожидание телеметрии...", status), 12, y)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.cfg.Window.Width, g.cfg.Window.Height
}
