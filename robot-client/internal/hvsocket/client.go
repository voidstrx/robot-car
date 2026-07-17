package hvsocket

import (
	"encoding/binary"
	"io"
	"log"
	"net"

	"github.com/Microsoft/go-winio"
)

// Connect подключается к WSL через Hyper-V Sockets
func Connect(vmName, serviceID string) (net.Conn, error) {
	pipePath := `\\.\pipe\WSL\` + vmName + `_` + serviceID

	conn, err := winio.DialPipe(pipePath, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// ReceiveRawFrame получает сырой кадр (BGR24) от WSL
func ReceiveRawFrame(conn net.Conn) ([]byte, int64, error) {
	header := make([]byte, 16)
	if _, err := io.ReadFull(conn, header); err != nil {
		log.Println("[hvsocket] Ошибка чтения заголовка:", err)
		return nil, 0, err
	}

	magic := binary.BigEndian.Uint32(header[0:4])
	if magic != 0xDEADBEEF {
		log.Printf("[hvsocket] Неверный magic: %x", magic)
		return nil, 0, io.ErrUnexpectedEOF
	}

	size := binary.BigEndian.Uint32(header[4:8])
	timestamp := int64(binary.BigEndian.Uint64(header[8:16]))

	rawData := make([]byte, size)
	if _, err := io.ReadFull(conn, rawData); err != nil {
		log.Println("[hvsocket] Ошибка чтения данных кадра:", err)
		return nil, 0, err
	}

	return rawData, timestamp, nil
}
