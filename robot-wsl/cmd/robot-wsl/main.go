package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"log"
	"os"
	"os/signal"
	"syscall"

	"robot-wsl/internal/grpc"
	"robot-wsl/internal/state"
	"robot-wsl/internal/video"
)

func main() {
	fmt.Println("robot-wsl — restructured")
	fmt.Println("========================")

	// === Конфиг ===
	cfgFile, err := os.ReadFile("configs/config.json")
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	var cfg struct {
		ServerAddr string `json:"server_addr"`
		RTSPUrl    string `json:"rtsp_url"`
	}
	if err := json.Unmarshal(cfgFile, &cfg); err != nil {
		log.Fatalf("config parse: %v", err)
	}
	if cfg.ServerAddr == "" {
		cfg.ServerAddr = "192.168.88.135:50051"
	}

	// === gRPC клиент к Raspberry Pi ===
	client := grpc.NewClient(cfg.ServerAddr)
	if err := client.Connect(); err != nil {
		log.Fatalf("gRPC connect: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Фоновые горутины Этапа 1
	client.StartModePolling(ctx)     // GetMode каждые 100 мс
	client.StartTelemetryStream(ctx) // StreamTelemetry

	log.Printf("Текущий режим: %s", state.Global.GetMode())

	// === Видео пайплайн + отправка в Windows ===
	pipeline := video.NewPipeline()

	// Create vsock sender to Windows (CID=2 is host, port 5000 as in original)
	sender, err := video.NewVsockVideoSender(2, 5000)
	if err != nil {
		log.Printf("[Video] Vsock sender not available (will run without sending to Windows): %v", err)
	} else {
		pipeline.Sender = sender
		defer sender.Close()
	}

	// Optional: custom OnFrame if you want extra processing before sending
	pipeline.OnFrame = func(frame *image.RGBA) {
		// overlay is already applied inside ProcessFrame
		// You can add extra logic here if needed
	}

	stopVideo := make(chan struct{})
	go pipeline.StartRTSPPipeline(cfg.RTSPUrl, stopVideo)

	log.Println("Всё запущено. Overlay (режим + телеметрия) рисуется на каждом кадре.")
	log.Println("Видео берётся по RTSP, overlay не перекрывает видео, кадры отправляются в Windows по vsock (port 5000).")

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Завершение...")
	close(stopVideo)
	cancel()
}
