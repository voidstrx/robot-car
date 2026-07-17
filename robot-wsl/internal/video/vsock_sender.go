package video

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"sync"
	"time"

	"github.com/mdlayher/vsock"
)

// VsockVideoSender sends processed video frames over vsock to Windows (Hyper-V).
// It uses ffmpeg to encode frames to mpegts on the fly for compatibility with the original client.
type VsockVideoSender struct {
	cid       uint32 // usually 2 for host from WSL
	port      uint32
	conn      net.Conn
	encoder   *exec.Cmd
	encoderIn io.WriteCloser  // stdin for raw BGR frames
	encoderOut io.ReadCloser // stdout mpegts
	sender    *bufio.Writer
	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
}

// NewVsockVideoSender creates sender that dials vsock to Windows.
// It will retry until the Windows client starts listening.
func NewVsockVideoSender(cid, port uint32) (*VsockVideoSender, error) {
	s := &VsockVideoSender{
		cid:    cid,
		port:   port,
		stopCh: make(chan struct{}),
	}

	// Retry dial until Windows client is listening
	var conn net.Conn
	var err error
	for i := 0; i < 30; i++ { // retry for ~30 seconds
		conn, err = vsock.Dial(s.cid, s.port, nil)
		if err == nil {
			break
		}
		log.Printf("[Video] Vsock dial attempt %d failed: %v (retrying...)", i+1, err)
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("vsock dial to CID %d port %d failed after retries: %w", cid, port, err)
	}

	s.conn = conn
	s.sender = bufio.NewWriter(conn)
	log.Printf("[Video] Vsock connected to Windows (CID=%d, port=%d)", cid, port)

	// Start ffmpeg encoder: rawvideo BGR24 stdin → mpegts stdout
	// Low latency settings
	encoder := exec.Command("ffmpeg",
		"-loglevel", "error",
		"-f", "rawvideo",
		"-pix_fmt", "bgr24",
		"-s", "1280x720",
		"-r", "30",
		"-i", "pipe:0",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-f", "mpegts",
		"-",
	)

	encoderIn, err := encoder.StdinPipe()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ffmpeg stdin: %w", err)
	}
	s.encoderIn = encoderIn

	encoderOut, err := encoder.StdoutPipe()
	if err != nil {
		encoderIn.Close()
		conn.Close()
		return nil, fmt.Errorf("ffmpeg stdout: %w", err)
	}
	s.encoderOut = encoderOut

	if err := encoder.Start(); err != nil {
		encoderIn.Close()
		encoderOut.Close()
		conn.Close()
		return nil, fmt.Errorf("start ffmpeg encoder: %w", err)
	}
	s.encoder = encoder

	s.running = true

	// Goroutine: read mpegts from encoder and send to vsock
	go s.sendLoop()

	log.Println("[Video] FFmpeg encoder started for vsock streaming")

	return s, nil
}

func (s *VsockVideoSender) sendLoop() {
	buf := make([]byte, 4096)
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		n, err := s.encoderOut.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("[Video] encoder read error: %v", err)
			}
			return
		}
		if n > 0 {
			s.mu.Lock()
			if s.sender != nil {
				_, _ = s.sender.Write(buf[:n])
				_ = s.sender.Flush()
			}
			s.mu.Unlock()
		}
	}
}

// SendFrame feeds a processed BGR frame to the encoder (called from OnFrame).
func (s *VsockVideoSender) SendFrame(bgr []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("sender not running")
	}

	// Try to reconnect if connection is lost
	if s.conn == nil {
		if err := s.reconnectLocked(); err != nil {
			return fmt.Errorf("reconnect failed: %w", err)
		}
	}

	_, err := s.encoderIn.Write(bgr)
	if err != nil {
		// Connection probably dead, try to reconnect next time
		s.conn = nil
		s.sender = nil
		return fmt.Errorf("write frame to encoder: %w", err)
	}
	return nil
}

// reconnectLocked tries to re-establish the vsock connection (must be called with lock held)
func (s *VsockVideoSender) reconnectLocked() error {
	// Close old connection if exists
	if s.conn != nil {
		_ = s.conn.Close()
	}

	for i := 0; i < 10; i++ {
		conn, err := vsock.Dial(s.cid, s.port, nil)
		if err == nil {
			s.conn = conn
			s.sender = bufio.NewWriter(conn)
			log.Printf("[Video] Vsock reconnected to Windows (CID=%d, port=%d)", s.cid, s.port)
			return nil
		}
		log.Printf("[Video] Reconnect attempt %d failed: %v", i+1, err)
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("failed to reconnect after 10 attempts")
}

// Close stops everything.
func (s *VsockVideoSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	close(s.stopCh)

	if s.encoderIn != nil {
		_ = s.encoderIn.Close()
	}
	if s.encoderOut != nil {
		_ = s.encoderOut.Close()
	}
	if s.encoder != nil && s.encoder.Process != nil {
		_ = s.encoder.Process.Kill()
	}
	if s.conn != nil {
		_ = s.conn.Close()
	}
	s.running = false
	log.Println("[Video] Vsock sender closed")
	return nil
}
