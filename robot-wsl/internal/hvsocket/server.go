package hvsocket

import (
	"log"
	"net"
	"sync"

	"github.com/mdlayher/vsock"
)

type Server struct {
	listener net.Listener
	conns    map[net.Conn]struct{}
	mu       sync.Mutex
}

func NewServer(port uint32) (*Server, error) {
	listener, err := vsock.Listen(port, nil)
	if err != nil {
		return nil, err
	}

	server := &Server{
		listener: listener,
		conns:    make(map[net.Conn]struct{}),
	}

	go server.acceptLoop()

	log.Printf("[hvsocket] vsock сервер запущен на порту %d", port)
	return server, nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Printf("[hvsocket] Accept error: %v", err)
			continue
		}

		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		log.Println("[hvsocket] Windows клиент подключился")
	}
}

// Send отправляет данные всем подключённым клиентам
func (s *Server) Send(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for conn := range s.conns {
		_, err := conn.Write(data)
		if err != nil {
			log.Printf("[hvsocket] Ошибка отправки: %v", err)
			conn.Close()
			delete(s.conns, conn)
		}
	}
}

func (s *Server) Close() error {
	return s.listener.Close()
}
