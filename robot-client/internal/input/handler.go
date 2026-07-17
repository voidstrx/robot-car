package input

import (
	"log"
	"net"
	"time"

	"github.com/0xcafed00d/joystick"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type Handler struct {
	conn         net.Conn // ← изменил здесь
	sendInterval time.Duration
	lastSend     time.Time
	joystick     joystick.Joystick
}

func NewHandler(conn net.Conn, interval time.Duration) *Handler { // ← изменил здесь
	js, err := joystick.Open(0)
	if err != nil {
		log.Println("[Input] Геймпад не найден, работаем только с клавиатурой")
		return &Handler{
			conn:         conn,
			sendInterval: interval,
			lastSend:     time.Now(),
		}
	}

	return &Handler{
		conn:         conn,
		sendInterval: interval,
		lastSend:     time.Now(),
		joystick:     js,
	}
}

func (h *Handler) Update() {}

func (h *Handler) SendIfNeeded() {
	if time.Since(h.lastSend) < h.sendInterval {
		return
	}

	state := h.buildInputState()
	data := state.Marshal()

	if _, err := h.conn.Write(data); err != nil {
		log.Println("[Input] Ошибка отправки InputState:", err)
	}

	h.lastSend = time.Now()
}

func (h *Handler) buildInputState() InputState {
	var state InputState

	// === Геймпад ===
	if h.joystick != nil {
		jinfo, err := h.joystick.Read()
		if err == nil {
			for i := 0; i < len(jinfo.AxisData) && i < len(state.Axes); i++ {
				state.Axes[i] = float32(jinfo.AxisData[i]) / 32768.0
			}
			state.Buttons = jinfo.Buttons
		}
	}

	// === Клавиатура ===
	pressedKeys := inpututil.AppendPressedKeys(nil)
	state.KeyCount = uint8(len(pressedKeys))
	for i := 0; i < len(pressedKeys) && i < len(state.Keys); i++ {
		state.Keys[i] = uint16(pressedKeys[i])
	}

	return state
}
