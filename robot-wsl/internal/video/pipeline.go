package video

import (
	"image"
	"log"
	"time"

	"robot-wsl/internal/state"
)

// FrameHandler — функция, которая вызывается на каждый готовый кадр
// (уже с наложенным overlay)
type FrameHandler func(frame *image.RGBA)

// Pipeline — простой цикл обработки видео
type Pipeline struct {
	OverlayCfg OverlayConfig
	OnFrame    FrameHandler
	Sender     *VsockVideoSender // optional vsock sender to Windows
}

func NewPipeline() *Pipeline {
	return &Pipeline{
		OverlayCfg: DefaultOverlay,
	}
}

// ProcessFrame принимает сырой кадр, накладывает overlay и отдаёт дальше.
// If Sender is set, also sends the BGR version over vsock.
func (p *Pipeline) ProcessFrame(frame *image.RGBA) {
	if frame == nil {
		return
	}

	// 1. Накладываем режим + телеметрию (верхняя панель, не перекрывает видео)
	DrawOverlay(frame, p.OverlayCfg)

	// 2. Отдаём готовый кадр (в Windows / на сохранение / и т.д.)
	if p.OnFrame != nil {
		p.OnFrame(frame)
	}

	// 3. Send to Windows via vsock (convert back to BGR for encoder)
	if p.Sender != nil {
		bgr := RGBAtoBGR(frame)
		_ = p.Sender.SendFrame(bgr) // ignore error for now, or log
	}
}

// StartMockPipeline — временная заглушка для теста overlay без реального RTSP.
// Генерирует чёрные кадры и обновляет телеметрию, чтобы было видно overlay.
func (p *Pipeline) StartMockPipeline(stop <-chan struct{}) {
	log.Println("[Video] Mock pipeline started (for overlay testing)")

	ticker := time.NewTicker(33 * time.Millisecond) // ~30 FPS
	defer ticker.Stop()

	frame := image.NewRGBA(image.Rect(0, 0, 1280, 720))

	for {
		select {
		case <-stop:
			log.Println("[Video] Mock pipeline stopped")
			return
		case <-ticker.C:
			// Очищаем кадр (чёрный)
			for i := range frame.Pix {
				frame.Pix[i] = 0
			}

			// Имитируем изменение телеметрии (чтобы было видно, что overlay обновляется)
			tel := state.Global.GetTelemetry()
			tel.Distance = 30 + float32(time.Now().UnixMilli()%200)/10
			state.Global.UpdateTelemetry(tel)

			p.ProcessFrame(frame)
		}
	}
}

// RGBAtoBGR converts image.RGBA to raw BGR24 bytes (for ffmpeg encoder).
func RGBAtoBGR(img *image.RGBA) []byte {
	bgr := make([]byte, len(img.Pix)/4*3)
	for i := 0; i < len(img.Pix); i += 4 {
		r := img.Pix[i]
		g := img.Pix[i+1]
		b := img.Pix[i+2]
		// BGR order
		j := (i / 4) * 3
		bgr[j] = b
		bgr[j+1] = g
		bgr[j+2] = r
	}
	return bgr
}

// StartRTSPPipeline starts real RTSP reception using ffmpeg, applies overlay on every frame
// and calls OnFrame with the processed RGBA image.
// Resolution is hardcoded to 1280x720 (matches original project).
func (p *Pipeline) StartRTSPPipeline(rtspURL string, stop <-chan struct{}) {
	log.Printf("[Video] Starting real RTSP pipeline from %s", rtspURL)

	reader, err := NewRTSPReader(rtspURL, 1280, 720)
	if err != nil {
		log.Fatalf("[Video] Failed to start RTSP reader: %v", err)
	}
	defer reader.Close()

	ticker := time.NewTicker(33 * time.Millisecond) // target ~30 FPS
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			log.Println("[Video] RTSP pipeline stopped")
			return
		case <-ticker.C:
			bgr, err := reader.ReadFrame()
			if err != nil {
				log.Printf("[Video] ReadFrame error: %v (reconnecting...)", err)
				time.Sleep(1 * time.Second)
				// Try to recreate reader
				reader.Close()
				reader, err = NewRTSPReader(rtspURL, 1280, 720)
				if err != nil {
					log.Printf("[Video] Reconnect failed: %v", err)
					continue
				}
				continue
			}

			// Convert BGR to RGBA for overlay
			frame := BGRToRGBA(bgr, 1280, 720)

			// Apply overlay (mode + telemetry) - does not cover main video
			p.ProcessFrame(frame)
		}
	}
}
