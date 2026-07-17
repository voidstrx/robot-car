package display

import (
	"log"
	"net"
	"sync"

	"robot-client/internal/hvsocket"

	"github.com/hajimehoshi/ebiten/v2"
	"gocv.io/x/gocv"
)

type FrameReceiver struct {
	conn   net.Conn // ← исправлено
	latest *ebiten.Image
	mu     sync.RWMutex
}

func NewFrameReceiver(conn net.Conn) *FrameReceiver { // ← исправлено
	fr := &FrameReceiver{conn: conn}
	go fr.receiveLoop()
	return fr
}

func (fr *FrameReceiver) receiveLoop() {
	log.Println("[FrameReceiver] Горутина запущена (raw BGR24)")

	for {
		rawData, ts, err := hvsocket.ReceiveRawFrame(fr.conn)
		if err != nil {
			log.Println("[FrameReceiver] Ошибка ReceiveRawFrame:", err)
			continue
		}

		log.Printf("[FrameReceiver] Получены сырые данные. Размер: %d байт, timestamp: %d", len(rawData), ts)

		img, err := gocv.NewMatFromBytes(720, 1280, gocv.MatTypeCV8UC3, rawData)
		if err != nil {
			log.Println("[FrameReceiver] Ошибка NewMatFromBytes:", err)
			continue
		}
		if img.Empty() {
			log.Println("[FrameReceiver] Mat пустой")
			img.Close()
			continue
		}

		goImg, err := img.ToImage()
		if err != nil {
			log.Println("[FrameReceiver] Ошибка ToImage:", err)
			img.Close()
			continue
		}

		ebitenImg := ebiten.NewImageFromImage(goImg)

		fr.mu.Lock()
		if fr.latest != nil {
			fr.latest.Dispose()
		}
		fr.latest = ebitenImg
		fr.mu.Unlock()

		img.Close()

		log.Println("[FrameReceiver] Кадр успешно обновлён")
	}
}

func (fr *FrameReceiver) GetLatestFrame() *ebiten.Image {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	return fr.latest
}
