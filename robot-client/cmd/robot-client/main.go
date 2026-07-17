package main

import (
	"encoding/json"

	"image"
	"image/color"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"robot-client/internal/grpc"
	pb "robot-client/internal/grpc/pb"
	"robot-client/internal/hvsock"
	"robot-client/internal/input"
)

const (
	screenW = 1280
	screenH = 720
	chatH   = 50
)

type Game struct {
	frameCh    <-chan *image.RGBA
	currentImg *ebiten.Image
	inputText  string
	status     string
	grpcClient *grpc.Client
	mode       pb.Mode
	modeMu     sync.RWMutex

	inputHandler *input.Handler
}

func NewGame(grpcClient *grpc.Client) *Game {
	cfg := input.DefaultInputConfig()
	// TODO: можно загрузить InputConfig из configs/input.json

	g := &Game{
		frameCh:      make(chan *image.RGBA, 3),
		status:       "Connecting to WSL...",
		grpcClient:   grpcClient,
		mode:         pb.Mode_MODE_MANUAL,
		inputHandler: input.NewHandler(cfg),
	}
	return g
}

func (g *Game) SetMode(m pb.Mode) {
	g.modeMu.Lock()
	g.mode = m
	g.modeMu.Unlock()
}

func (g *Game) GetMode() pb.Mode {
	g.modeMu.RLock()
	defer g.modeMu.RUnlock()
	return g.mode
}

func (g *Game) Update() error {
	// Receive video frames from Hvsock
	select {
	case img := <-g.frameCh:
		if g.currentImg == nil {
			g.currentImg = ebiten.NewImage(screenW, screenH)
		}
		g.currentImg.WritePixels(img.Pix)
		g.status = "Video OK (from WSL)"
	default:
	}

	// Text input
	g.inputText += string(ebiten.AppendInputChars(nil))
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(g.inputText) > 0 {
		g.inputText = g.inputText[:len(g.inputText)-1]
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		cmd := strings.TrimSpace(g.inputText)
		if cmd != "" {
			if strings.HasPrefix(cmd, "/mode ") {
				modeStr := strings.ToLower(strings.TrimSpace(cmd[6:]))
				if modeStr == "manual" || modeStr == "1" {
					g.grpcClient.SetMode(pb.Mode_MODE_MANUAL)
					g.SetMode(pb.Mode_MODE_MANUAL)
					g.status = "Mode → MANUAL"
				} else if modeStr == "auto" || modeStr == "2" {
					g.grpcClient.SetMode(pb.Mode_MODE_AUTO)
					g.SetMode(pb.Mode_MODE_AUTO)
					g.status = "Mode → AUTO"
				}
			} else {
				g.grpcClient.SetMessage(cmd)
				g.status = "Message sent"
			}
			g.inputText = ""
		}
	}

	// === Управление (клавиатура + геймпад) только в MANUAL режиме ===
	if g.GetMode() == pb.Mode_MODE_MANUAL {
		g.inputHandler.Update()

		steering := float32(g.inputHandler.GetSteering())
		move := float32(g.inputHandler.GetMove())
		pan := float32(g.inputHandler.GetPan())
		tilt := float32(g.inputHandler.GetTilt())

		// Отправляем команду, если есть хоть какое-то движение
		if steering != 0 || move != 0 || pan != 0 || tilt != 0 {
			g.grpcClient.SendCommand(steering, move, pan, tilt)
		}

		// Кнопка STOP
		if g.inputHandler.IsStopPressed() {
			g.grpcClient.SendCommand(0, 0, 0, 0)
			g.status = "STOP sent"
		}
	}

	// Quick mode switch with M key
	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		if g.GetMode() == pb.Mode_MODE_MANUAL {
			g.grpcClient.SetMode(pb.Mode_MODE_AUTO)
			g.SetMode(pb.Mode_MODE_AUTO)
			g.status = "AUTO mode"
		} else {
			g.grpcClient.SetMode(pb.Mode_MODE_MANUAL)
			g.SetMode(pb.Mode_MODE_MANUAL)
			g.status = "MANUAL mode"
		}
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{20, 20, 20, 255})

	if g.currentImg != nil {
		op := &ebiten.DrawImageOptions{}
		screen.DrawImage(g.currentImg, op)
	} else {
		ebitenutil.DebugPrintAt(screen, "Waiting for video stream from WSL...", 20, 20)
	}

	// Bottom bar
	ebitenutil.DrawRect(screen, 0, float64(screenH), float64(screenW), float64(chatH), color.RGBA{0, 0, 0, 220})
	ebitenutil.DebugPrintAt(screen, "> "+g.inputText+"_", 12, screenH+12)
	ebitenutil.DebugPrintAt(screen, g.status, 12, 20)

	modeStr := "MANUAL"
	if g.GetMode() == pb.Mode_MODE_AUTO {
		modeStr = "AUTO"
	}
	ebitenutil.DebugPrintAt(screen, "Mode: "+modeStr+"  [M] switch  [Enter] msg  [Gamepad supported]", screenW-320, 20)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenW, screenH + chatH
}

func main() {
	cfgFile, err := os.ReadFile("configs/config.json")
	if err != nil {
		log.Fatalf("config read error: %v", err)
	}

	var cfg struct {
		RaspberryPiAddr string `json:"raspberry_pi_addr"`
		WSLVMGUID       string `json:"wsl_vm_guid"` // ← real Hyper-V VM GUID, e.g. "f7a1a771-33da-4dcd-bde1-3bd4d26af672"
		HvSocket        struct {
			VideoServiceID string `json:"video_service_id"`
		} `json:"hvsocket"`
		Window struct {
			Width  int    `json:"width"`
			Height int    `json:"height"`
			Title  string `json:"title"`
		} `json:"window"`
	}
	json.Unmarshal(cfgFile, &cfg)

	if cfg.RaspberryPiAddr == "" {
		cfg.RaspberryPiAddr = "192.168.88.135:50051"
	}
	if cfg.Window.Width == 0 {
		cfg.Window.Width = screenW
		cfg.Window.Height = screenH
	}

	// gRPC to Pi
	grpcClient := grpc.NewClient(cfg.RaspberryPiAddr)
	if err := grpcClient.Connect(); err != nil {
		log.Printf("gRPC to Pi warning: %v", err)
	}

	// Hvsock video receiver from WSL
	videoRecv, err := hvsock.NewVideoReceiver(
		cfg.WSLVMGUID, // ← must be the real Hyper-V VM GUID (not the friendly name)
		cfg.HvSocket.VideoServiceID,
		cfg.Window.Width,
		cfg.Window.Height,
	)
	if err != nil {
		log.Fatalf("video receiver error: %v", err)
	}
	videoRecv.Start()

	game := NewGame(grpcClient)
	game.frameCh = videoRecv.Frames()

	ebiten.SetWindowSize(cfg.Window.Width, cfg.Window.Height+chatH)
	ebiten.SetWindowTitle(cfg.Window.Title)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}

	videoRecv.Stop()
}
