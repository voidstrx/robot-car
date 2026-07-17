package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/mdlayher/vsock"
)

const (
	videoPort   = 5000
	commandPort = 5001
	rtspURL     = "rtsp://192.168.88.135:8554/robot"
)

func sendVideo() {
	for {
		fmt.Println("[VIDEO] Connecting to Windows...")
		conn, err := vsock.Dial(2, videoPort, nil)
		if err != nil {
			log.Printf("[VIDEO] Dial failed: %v (retry in 2s)", err)
			time.Sleep(2 * time.Second)
			continue
		}
		fmt.Println("[VIDEO] Connected to Windows")

		cmd := exec.Command("ffmpeg",
			"-loglevel", "warning",
			"-rtsp_transport", "tcp",
			"-fflags", "nobuffer",
			"-flags", "low_delay",
			"-i", rtspURL,
			"-c", "copy",
			"-f", "mpegts",
			"-an",
			"-",
		)
		cmd.Stderr = os.Stderr

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			conn.Close()
			time.Sleep(2 * time.Second)
			continue
		}
		if err := cmd.Start(); err != nil {
			conn.Close()
			log.Println(err)
			time.Sleep(2 * time.Second)
			continue
		}

		_, err = io.Copy(conn, stdout)
		fmt.Println("[VIDEO] Stream ended:", err)

		cmd.Process.Kill()
		conn.Close()
		time.Sleep(1 * time.Second)
	}
}

func handleCommands() {
	for {
		fmt.Println("[CMD] Listening on port", commandPort)
		listener, err := vsock.Listen(commandPort, nil)
		if err != nil {
			log.Printf("[CMD] Listen error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		conn, err := listener.Accept()
		if err != nil {
			listener.Close()
			continue
		}
		fmt.Println("[CMD] Windows connected")

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			fmt.Println("[CMD received]", scanner.Text())
		}

		conn.Close()
		listener.Close()
		fmt.Println("[CMD] Disconnected")
	}
}

func main() {
	go handleCommands()
	sendVideo()
}
