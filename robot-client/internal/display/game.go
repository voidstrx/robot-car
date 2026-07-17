package display

import (
	"log"
	"robot-client/internal/input"

	"github.com/hajimehoshi/ebiten/v2"
)

type Game struct {
	frameReceiver *FrameReceiver
	inputHandler  *input.Handler
	lastFrame     *ebiten.Image
}

func NewGame(receiver *FrameReceiver, handler *input.Handler) *Game {
	return &Game{
		frameReceiver: receiver,
		inputHandler:  handler,
	}
}

func (g *Game) Update() error {
	g.inputHandler.Update()
	g.inputHandler.SendIfNeeded()
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	frame := g.frameReceiver.GetLatestFrame()
	if frame != nil {
		g.lastFrame = frame
		log.Println("[Game] Рисую кадр") // временно
	} else {
		log.Println("[Game] Кадра пока нет")
	}

	if g.lastFrame != nil {
		screen.DrawImage(g.lastFrame, nil)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return 1280, 720 // Фиксированный логический размер
}
