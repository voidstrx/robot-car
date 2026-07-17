package video

import (
	"bufio"
	"fmt"
	"image"
	"io"
	"log"
	"os/exec"
	"sync"
)

// RTSPReader pulls video from RTSP URL using ffmpeg and provides raw BGR frames.
type RTSPReader struct {
	url       string
	width     int
	height    int
	frameSize int
	cmd       *exec.Cmd
	stdout    io.ReadCloser
	reader    *bufio.Reader
	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
}

// NewRTSPReader creates a new RTSP reader.
// Default resolution 1280x720 (can be changed).
func NewRTSPReader(url string, width, height int) (*RTSPReader, error) {
	if width == 0 {
		width = 1280
	}
	if height == 0 {
		height = 720
	}

	r := &RTSPReader{
		url:       url,
		width:     width,
		height:    height,
		frameSize: width * height * 3, // BGR24
		stopCh:    make(chan struct{}),
	}

	// Build ffmpeg command for low-latency RTSP pull
	// -rtsp_transport tcp for reliability
	// -fflags nobuffer + low_delay for minimal latency
	args := []string{
		"-loglevel", "error",
		"-rtsp_transport", "tcp",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-i", url,
		"-f", "rawvideo",
		"-pix_fmt", "bgr24",
		"-an", // no audio
		"-",
	}

	r.cmd = exec.Command("ffmpeg", args...)
	stdout, err := r.cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}
	r.stdout = stdout
	r.reader = bufio.NewReaderSize(stdout, r.frameSize*2)

	if err := r.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	r.running = true
	log.Printf("[Video] RTSP reader started for %s (%dx%d)", url, width, height)

	// Monitor process
	go r.monitor()

	return r, nil
}

func (r *RTSPReader) monitor() {
	err := r.cmd.Wait()
	r.mu.Lock()
	r.running = false
	r.mu.Unlock()
	if err != nil && err.Error() != "signal: killed" {
		log.Printf("[Video] ffmpeg exited with error: %v", err)
	}
}

// ReadFrame reads one raw BGR frame.
// Blocks until frame is available or error.
func (r *RTSPReader) ReadFrame() ([]byte, error) {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return nil, fmt.Errorf("reader not running")
	}
	r.mu.Unlock()

	buf := make([]byte, r.frameSize)
	_, err := io.ReadFull(r.reader, buf)
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("stream ended")
		}
		return nil, err
	}
	return buf, nil
}

// Close stops the reader and ffmpeg.
func (r *RTSPReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil
	}

	close(r.stopCh)
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
	if r.stdout != nil {
		_ = r.stdout.Close()
	}
	r.running = false
	log.Println("[Video] RTSP reader closed")
	return nil
}

// ToRGBA converts BGR24 raw bytes to image.RGBA (for overlay).
func BGRToRGBA(bgr []byte, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(bgr); i += 3 {
		b := bgr[i]
		g := bgr[i+1]
		r := bgr[i+2]

		// RGBA layout: R G B A
		idx := (i / 3) * 4
		img.Pix[idx] = r
		img.Pix[idx+1] = g
		img.Pix[idx+2] = b
		img.Pix[idx+3] = 255
	}
	return img
}
