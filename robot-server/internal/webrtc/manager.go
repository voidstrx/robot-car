package webrtc

import (
	"log"
	"net"
	"os/exec"
	"strconv"
	"sync"

	pb "robot-server/internal/grpc/pb"

	"github.com/pion/webrtc/v3"
)

type VideoConfig struct {
	Enabled     bool `json:"enabled"`
	Width       int  `json:"width"`
	Height      int  `json:"height"`
	FPS         int  `json:"fps"`
	BitrateKbps int  `json:"bitrate_kbps"`
	RTPPort     int  `json:"rtp_port"`
}

type Config struct {
	Enabled bool        `json:"enabled"`
	Video   VideoConfig `json:"video"`
	Audio   struct {
		Enabled bool `json:"enabled"`
	} `json:"audio"`
}

type Manager struct {
	mu             sync.Mutex
	peerConnection *webrtc.PeerConnection
	signalStream   pb.RobotControl_WebRTCSignalingServer

	videoTrack *webrtc.TrackLocalStaticRTP
	config     Config

	videoCmd *exec.Cmd
	udpConn  *net.UDPConn
}

func NewManager(cfg Config) *Manager {
	return &Manager{config: cfg}
}

func (m *Manager) HandleSignaling(stream pb.RobotControl_WebRTCSignalingServer) error {
	m.mu.Lock()
	m.signalStream = stream
	m.mu.Unlock()

	log.Println("[WebRTC] Signaling stream подключён")

	for {
		signal, err := stream.Recv()
		if err != nil {
			return err
		}

		switch payload := signal.Payload.(type) {
		case *pb.WebRTCSignal_Offer:
			m.handleOffer(payload.Offer)
		case *pb.WebRTCSignal_IceCandidate:
			m.handleRemoteICECandidate(payload.IceCandidate)
		case *pb.WebRTCSignal_StartWebrtc:
			m.startMediaStreaming()
		}
	}
}

func (m *Manager) handleOffer(offerSDP string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Println("[WebRTC] Получен Offer от клиента")

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		log.Println("[WebRTC] Ошибка создания PeerConnection:", err)
		return
	}
	m.peerConnection = pc

	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("[WebRTC] Connection State: %s", s.String())
	})

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		m.sendSignal(&pb.WebRTCSignal{
			Payload: &pb.WebRTCSignal_IceCandidate{
				IceCandidate: candidate.ToJSON().Candidate,
			},
		})
	})

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}); err != nil {
		log.Println("[WebRTC] SetRemoteDescription error:", err)
		return
	}

	// Запускаем медиа синхронно (без go), до CreateAnswer
	m.startMediaStreaming()

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Println("[WebRTC] CreateAnswer error:", err)
		return
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		log.Println("[WebRTC] SetLocalDescription error:", err)
		return
	}

	m.sendSignal(&pb.WebRTCSignal{
		Payload: &pb.WebRTCSignal_Answer{Answer: answer.SDP},
	})

	log.Println("[WebRTC] Answer отправлен клиенту")
}

func (m *Manager) handleRemoteICECandidate(candidate string) {
	if m.peerConnection != nil {
		m.peerConnection.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate})
	}
}

func (m *Manager) sendSignal(signal *pb.WebRTCSignal) {
	if m.signalStream != nil {
		m.signalStream.Send(signal)
	}
}

func (m *Manager) startMediaStreaming() {
	if !m.config.Video.Enabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.videoTrack != nil {
		return
	}

	log.Println("[WebRTC] Запуск видео источника (rpicam-vid)...")

	// Создаём transceiver для отправки видео
	transceiver, err := m.peerConnection.AddTransceiverFromKind(
		webrtc.RTPCodecTypeVideo,
		webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionSendonly,
		},
	)
	if err != nil {
		log.Println("[WebRTC] AddTransceiver error:", err)
		return
	}

	// Создаём трек
	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"pion",
	)
	if err != nil {
		log.Println("[WebRTC] Ошибка создания VideoTrack:", err)
		return
	}
	m.videoTrack = track

	// Привязываем трек к отправителю
	if err := transceiver.Sender().ReplaceTrack(track); err != nil {
		log.Println("[WebRTC] ReplaceTrack error:", err)
		return
	}

	go m.startLibcameraVid()
	go m.forwardRTPToWebRTC()
}

func (m *Manager) startLibcameraVid() {
	width := strconv.Itoa(m.config.Video.Width)
	height := strconv.Itoa(m.config.Video.Height)
	fps := strconv.Itoa(m.config.Video.FPS)
	bitrate := strconv.Itoa(m.config.Video.BitrateKbps * 1000)
	port := strconv.Itoa(m.config.Video.RTPPort)

	cmd := exec.Command("rpicam-vid",
		"--width", width,
		"--height", height,
		"--framerate", fps,
		"--codec", "h264",
		"--inline",
		"--bitrate", bitrate,
		"-t", "0",
		"--output", "udp://127.0.0.1:"+port,
	)

	m.videoCmd = cmd

	log.Printf("[WebRTC] Запуск: rpicam-vid %dx%d@%s fps → UDP:%s",
		m.config.Video.Width, m.config.Video.Height, fps, port)

	if err := cmd.Start(); err != nil {
		log.Println("[WebRTC] Ошибка запуска rpicam-vid:", err)
	}
}

func (m *Manager) forwardRTPToWebRTC() {
	addr := &net.UDPAddr{Port: m.config.Video.RTPPort}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Println("[WebRTC] UDP listen error:", err)
		return
	}
	m.udpConn = conn
	defer conn.Close()

	log.Printf("[WebRTC] Слушаем RTP на порту %d", m.config.Video.RTPPort)

	buf := make([]byte, 1500)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if m.videoTrack != nil {
			m.videoTrack.Write(buf[:n])
		}
	}
}

func (m *Manager) Stop() {
	if m.videoCmd != nil && m.videoCmd.Process != nil {
		m.videoCmd.Process.Kill()
	}
	if m.udpConn != nil {
		m.udpConn.Close()
	}
}
