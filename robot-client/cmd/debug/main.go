package main

import (
	"log"
	"time"

	"robot-client/internal/display"
	"robot-client/internal/hvsocket"
	"robot-client/internal/input"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	cfg := LoadConfig("configs/client.json")

	// === Подключаемся к WSL (два соединения) ===
	videoConn, err := hvsocket.Connect(cfg.WSLHost + ":" + cfg.VideoPort)
	if err != nil {
		log.Fatalf("Не удалось подключиться к видео WSL: %v", err)
	}

	commandConn, err := hvsocket.Connect(cfg.WSLHost + ":" + cfg.CommandPort)
	if err != nil {
		log.Fatalf("Не удалось подключиться к командам WSL: %v", err)
	}

	log.Println("[Client] Подключено к WSL")

	// === Инициализация ===
	frameReceiver := display.NewFrameReceiver(videoConn)
	inputHandler := input.NewHandler(commandConn, 16*time.Millisecond) // ~60 FPS

	game := display.NewGame(frameReceiver, inputHandler)

	// === Запуск Ebiten ===
	ebiten.SetWindowSize(cfg.Window.Width, cfg.Window.Height)
	ebiten.SetWindowTitle(cfg.Window.Title)
	ebiten.SetVsyncEnabled(true)

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
