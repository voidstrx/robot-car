package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"net"
	"time"
)

func main() {
	ln, _ := net.Listen("tcp", ":50051")
	defer ln.Close()

	log.Println("[WSL] Ожидание подключения...")
	conn, _ := ln.Accept()
	defer conn.Close()

	log.Println("[WSL] Windows подключён")

	for {
		// Создаём тестовый кадр
		img := image.NewRGBA(image.Rect(0, 0, 640, 480))
		for y := 0; y < 480; y++ {
			for x := 0; x < 640; x++ {
				img.Set(x, y, color.RGBA{0, uint8((x + y) % 255), 100, 255})
			}
		}

		var buf bytes.Buffer
		jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
		jpegData := buf.Bytes()

		// Заголовок
		header := make([]byte, 16)
		binary.BigEndian.PutUint32(header[0:4], 0xDEADBEEF)
		binary.BigEndian.PutUint32(header[4:8], uint32(len(jpegData)))
		binary.BigEndian.PutUint64(header[8:16], uint64(time.Now().UnixNano()))

		conn.Write(header)
		conn.Write(jpegData)

		time.Sleep(33 * time.Millisecond)
	}
}
