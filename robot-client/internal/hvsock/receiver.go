package hvsock

import (
	"fmt"
	"image"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
)

// VideoReceiver listens on Hyper-V socket and decodes video stream using ffmpeg.
// It provides RGBA frames ready for Ebiten.
type VideoReceiver struct {
	vmID        guid.GUID
	serviceID   guid.GUID
	width       int
	height      int
	frameCh     chan *image.RGBA
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

func NewVideoReceiver(vmIDStr, serviceIDStr string, width, height int) (*VideoReceiver, error) {
	vmID, err := guid.FromString(vmIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid VMID: %w", err)
	}
	serviceID, err := guid.FromString(serviceIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid serviceID: %w", err)
	}

	return &VideoReceiver{
		vmID:      vmID,
		serviceID: serviceID,
		width:     width,
		height:    height,
		frameCh:   make(chan *image.RGBA, 3),
		stopCh:    make(chan struct{}),
	}, nil
}

func (r *VideoReceiver) Start() {
	r.wg.Add(1)
	go r.run()
}

func (r *VideoReceiver) run() {
	defer r.wg.Done()

	for {
		select {
		case <-r.stopCh:
			return
		default:
		}

		addr := &winio.HvsockAddr{
			VMID:      r.vmID,
			ServiceID: winio.VsockServiceID(5000),
		}

		listener, err := winio.ListenHvsock(addr)
		if err != nil {
			log.Printf("[Hvsock] Listen error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		log.Println("[Hvsock] Waiting for WSL video connection...")

		conn, err := listener.Accept()
		if err != nil {
			listener.Close()
			continue
		}
		log.Println("[Hvsock] Video connected from WSL")

		// Spawn ffmpeg to decode mpegts/rawvideo from WSL into raw BGR
		cmd := exec.Command("ffmpeg",
			"-loglevel", "error",
			"-fflags", "nobuffer",
			"-flags", "low_delay",
			"-i", "pipe:0",
			"-f", "rawvideo",
			"-pix_fmt", "bgr24",
			"-s", fmt.Sprintf("%dx%d", r.width, r.height),
			"-an",
			"-",
		)

		stdin, _ := cmd.StdinPipe()
		stdout, _ := cmd.StdoutPipe()
		if err := cmd.Start(); err != nil {
			conn.Close()
			listener.Close()
			log.Printf("[Hvsock] ffmpeg start error: %v", err)
			continue
		}

		// Copy from vsock to ffmpeg stdin
		go func() {
			io.Copy(stdin, conn)
			stdin.Close()
		}()

		frameSize := r.width * r.height * 3
		buf := make([]byte, frameSize)

		for {
			select {
			case <-r.stopCh:
				cmd.Process.Kill()
				conn.Close()
				listener.Close()
				return
			default:
			}

			if _, err := io.ReadFull(stdout, buf); err != nil {
				break
			}

			// Convert BGR to RGBA for Ebiten
			img := image.NewRGBA(image.Rect(0, 0, r.width, r.height))
			for i, j := 0, 0; i < frameSize; i, j = i+3, j+4 {
				img.Pix[j+0] = buf[i+2] // R
				img.Pix[j+1] = buf[i+1] // G
				img.Pix[j+2] = buf[i+0] // B
				img.Pix[j+3] = 255
			}

			select {
			case r.frameCh <- img:
			default:
				// drop frame if channel full
			}
		}

		cmd.Process.Kill()
		conn.Close()
		listener.Close()
		log.Println("[Hvsock] Video disconnected, reconnecting...")
		time.Sleep(1 * time.Second)
	}
}

func (r *VideoReceiver) Frames() <-chan *image.RGBA {
	return r.frameCh
}

func (r *VideoReceiver) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}
