package main

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font/basicfont"
)

const (
	videoPort   = 5000
	commandPort = 5001
	screenW     = 1280
	screenH     = 720
	chatH       = 50
)

var (
	commandConn io.WriteCloser
	commandMu   sync.Mutex
)

func getVMID() guid.GUID {
	// ========== АКТУАЛЬНЫЙ VMID ==========
	id, err := guid.FromString("f7a1a771-33da-4dcd-bde1-3bd4d26af672")
	if err != nil {
		log.Fatal(err)
	}
	return id
}

func startVideoReceiver(frameCh chan<- *image.RGBA) {
	vmID := getVMID()

	for {
		addr := &winio.HvsockAddr{
			VMID:      vmID,
			ServiceID: winio.VsockServiceID(videoPort),
		}

		listener, err := winio.ListenHvsock(addr)
		if err != nil {
			log.Printf("[VIDEO] Listen: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		fmt.Println("[VIDEO] Waiting for WSL...")

		conn, err := listener.Accept()
		if err != nil {
			listener.Close()
			continue
		}
		fmt.Println("[VIDEO] Connected")

		cmd := exec.Command("ffmpeg",
			"-loglevel", "error",
			"-fflags", "nobuffer",
			"-flags", "low_delay",
			"-probesize", "32",
			"-analyzeduration", "0",
			"-i", "pipe:0",
			"-f", "rawvideo",
			"-pix_fmt", "bgr24",
			"-s", fmt.Sprintf("%dx%d", screenW, screenH),
			"-an",
			"-",
		)

		stdin, _ := cmd.StdinPipe()
		stdout, _ := cmd.StdoutPipe()
		cmd.Start()

		go func() {
			io.Copy(stdin, conn)
			stdin.Close()
		}()

		frameSize := screenW * screenH * 3
		buf := make([]byte, frameSize)

		for {
			if _, err := io.ReadFull(stdout, buf); err != nil {
				break
			}

			img := image.NewRGBA(image.Rect(0, 0, screenW, screenH))
			for i, j := 0, 0; i < frameSize; i, j = i+3, j+4 {
				img.Pix[j+0] = buf[i+2]
				img.Pix[j+1] = buf[i+1]
				img.Pix[j+2] = buf[i+0]
				img.Pix[j+3] = 255
			}

			select {
			case frameCh <- img:
			default:
			}
		}

		cmd.Process.Kill()
		conn.Close()
		listener.Close()
		fmt.Println("[VIDEO] Disconnected, reconnecting...")
		time.Sleep(1 * time.Second)
	}
}

func sendCommand(cmd string) {
	commandMu.Lock()
	defer commandMu.Unlock()
	if commandConn != nil {
		fmt.Fprintln(commandConn, cmd)
		fmt.Println("[CMD sent]", cmd)
	} else {
		fmt.Println("[CMD] no connection:", cmd)
	}
}

type Game struct {
	frameCh    chan *image.RGBA
	currentImg *ebiten.Image
	inputText  string
	status     string
}

func NewGame() *Game {
	return &Game{
		frameCh: make(chan *image.RGBA, 3),
		status:  "Waiting for video...",
	}
}

func (g *Game) Update() error {
	select {
	case img := <-g.frameCh:
		if g.currentImg == nil {
			g.currentImg = ebiten.NewImage(screenW, screenH)
			g.status = "Video OK"
		}
		g.currentImg.WritePixels(img.Pix)
	default:
	}

	// Ввод текста
	g.inputText += string(ebiten.AppendInputChars(nil))
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(g.inputText) > 0 {
		g.inputText = g.inputText[:len(g.inputText)-1]
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		cmd := strings.TrimSpace(g.inputText)
		if cmd != "" {
			sendCommand(cmd)
			g.status = "Sent: " + cmd
			g.inputText = ""
		}
	}

	// Клавиши
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		sendCommand("forward")
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		sendCommand("backward")
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		sendCommand("left")
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		sendCommand("right")
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		sendCommand("stop")
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{20, 20, 20, 255})

	// Видео
	if g.currentImg != nil {
		op := &ebiten.DrawImageOptions{}
		screen.DrawImage(g.currentImg, op)
	} else {
		ebitenutil.DebugPrintAt(screen, "No video yet", 20, 20)
	}

	// Чат внизу
	ebitenutil.DrawRect(screen, 0, float64(screenH), float64(screenW), float64(chatH), color.RGBA{0, 0, 0, 220})
	text.Draw(screen, "> "+g.inputText+"_", basicfont.Face7x13, 12, screenH+32, color.White)
	text.Draw(screen, g.status, basicfont.Face7x13, 12, 20, color.RGBA{0, 255, 100, 255})
}

func (g *Game) Layout(w, h int) (int, int) {
	return screenW, screenH + chatH
}

func main() {
	game := NewGame()
	go startVideoReceiver(game.frameCh)

	ebiten.SetWindowSize(screenW, screenH+chatH)
	ebiten.SetWindowTitle("Robot Control + Chat")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
