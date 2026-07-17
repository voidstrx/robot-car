package video

import (
	"log"
	"sync"

	"gocv.io/x/gocv"
)

type RTSPStream struct {
	url     string
	capture *gocv.VideoCapture
	frame   gocv.Mat
	mu      sync.RWMutex
	stopCh  chan struct{}
	running bool
}

func NewRTSPStream(url string) *RTSPStream {
	return &RTSPStream{
		url:    url,
		frame:  gocv.NewMat(),
		stopCh: make(chan struct{}),
	}
}

func (s *RTSPStream) Start() error {
	cap, err := gocv.OpenVideoCapture(s.url)
	if err != nil {
		return err
	}
	s.capture = cap
	s.running = true

	go s.run()
	return nil
}

func (s *RTSPStream) run() {
	for {
		select {
		case <-s.stopCh:
			return
		default:
			if s.capture == nil {
				return
			}
			if ok := s.capture.Read(&s.frame); !ok {
				log.Println("[RTSP] Не удалось прочитать кадр")
				return
			}
		}
	}
}

// GetFrame — для отрисовки в Ebiten
func (s *RTSPStream) GetFrame() gocv.Mat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.frame.Clone()
}

// GetFrameForAI — для будущего ИИ (возвращает оригинальный кадр без копирования)
func (s *RTSPStream) GetFrameForAI() gocv.Mat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.frame
}

func (s *RTSPStream) Stop() {
	if s.running {
		close(s.stopCh)
		s.running = false
	}
	if s.capture != nil {
		s.capture.Close()
	}
	s.frame.Close()
}
