package webrtc

import (
	"image"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/pion/webrtc/v3"

	pb "robot-client/internal/grpc/pb"
)

type Client struct {
	SignalStream pb.RobotControl_WebRTCSignalingClient
	Conn         *webrtc.PeerConnection

	Status      string
	VideoStatus string

	currentFrame *ebiten.Image
	frameMutex   sync.Mutex
}

func NewClient() *Client {
	return &Client{
		Status:      "Connecting...",
		VideoStatus: "Ожидание",
	}
}

func (c *Client) Start(signalStream pb.RobotControl_WebRTCSignalingClient) {
	c.SignalStream = signalStream
	c.createPeerConnection()
}

func (c *Client) createPeerConnection() {
	c.Status = "Creating PeerConnection..."

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		c.Status = "Error"
		log.Println("WebRTC error:", err)
		return
	}
	c.Conn = pc

	// Заявляем, что будем принимать видео
	_, _ = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})

	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		c.Status = "State: " + s.String()
		log.Printf("WebRTC Connection State: %s", s.String())

		if s == webrtc.PeerConnectionStateConnected {
			// Как только соединение установлено — запускаем приём видео
			go c.startVideoReceiving()
		}
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf(">>> Получен трек: %s", track.Kind())
		if track.Kind() == webrtc.RTPCodecTypeVideo {
			c.Status = "Video track received"
			go c.handleVideoTrack(track)
		}
	})

	time.Sleep(100 * time.Millisecond)

	offer, _ := pc.CreateOffer(nil)
	pc.SetLocalDescription(offer)

	c.SignalStream.Send(&pb.WebRTCSignal{
		Payload: &pb.WebRTCSignal_Offer{Offer: offer.SDP},
	})

	c.Status = "Offer sent"
	log.Println("WebRTC Offer отправлен")

	go c.signalLoop()
}

func (c *Client) signalLoop() {
	for {
		signal, err := c.SignalStream.Recv()
		if err != nil {
			return
		}

		if answer, ok := signal.Payload.(*pb.WebRTCSignal_Answer); ok {
			c.Conn.SetRemoteDescription(webrtc.SessionDescription{
				Type: webrtc.SDPTypeAnswer,
				SDP:  answer.Answer,
			})
			log.Println("[CLIENT] Получен Answer от сервера")
		}
	}
}

// Запускается при переходе соединения в connected
func (c *Client) startVideoReceiving() {
	c.VideoStatus = "Waiting for track..."

	// Пробуем найти трек в течение 3 секунд
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)

		// Если OnTrack уже сработал — выходим
		if c.VideoStatus == "Receiving frames" || c.VideoStatus == "ffmpeg running" {
			return
		}
	}

	// Если трек так и не пришёл через OnTrack — всё равно пробуем запустить ffmpeg
	// (временный fallback)
	log.Println("[CLIENT] OnTrack не сработал, запускаем ffmpeg в любом случае")
	c.VideoStatus = "Starting ffmpeg (fallback)"
	// Здесь можно добавить ручной запуск ffmpeg, если нужно
}

func (c *Client) handleVideoTrack(track *webrtc.TrackRemote) {
	c.VideoStatus = "Starting ffmpeg..."

	cmd := exec.Command("ffmpeg",
		"-f", "h264",
		"-i", "pipe:0",
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"-",
	)

	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()

	if err := cmd.Start(); err != nil {
		c.VideoStatus = "ffmpeg error"
		return
	}

	c.VideoStatus = "ffmpeg running"
	log.Println("[CLIENT] ffmpeg запущен успешно")

	go func() {
		defer stdin.Close()
		for {
			pkt, _, err := track.ReadRTP()
			if err != nil {
				return
			}
			stdin.Write(pkt.Payload)
		}
	}()

	go c.decodeFrames(stdout)
}

func (c *Client) decodeFrames(stdout io.ReadCloser) {
	width, height := 640, 480
	frameSize := width * height * 4
	buf := make([]byte, frameSize)

	for {
		_, err := io.ReadFull(stdout, buf)
		if err != nil {
			return
		}

		img := &image.RGBA{
			Pix:    buf,
			Stride: width * 4,
			Rect:   image.Rect(0, 0, width, height),
		}

		c.frameMutex.Lock()
		c.currentFrame = ebiten.NewImageFromImage(img)
		c.frameMutex.Unlock()
	}
}

func (c *Client) Draw(screen *ebiten.Image) {
	c.frameMutex.Lock()
	if c.currentFrame != nil {
		screen.DrawImage(c.currentFrame, nil)
	}
	c.frameMutex.Unlock()
}

func (c *Client) GetStatuses() (string, string) {
	return c.Status, c.VideoStatus
}
