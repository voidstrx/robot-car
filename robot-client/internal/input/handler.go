package input

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Handler управляет вводом (клавиатура + геймпад)
type Handler struct {
	cfg        InputConfig
	lastGamepad ebiten.GamepadID
}

// NewHandler создаёт обработчик ввода
func NewHandler(cfg InputConfig) *Handler {
	return &Handler{
		cfg: cfg,
	}
}

// Update должен вызываться каждый кадр
func (h *Handler) Update() {
	// Обновляем текущий геймпад (берём первый подключённый)
	ids := ebiten.GamepadIDs()
	if len(ids) > 0 {
		h.lastGamepad = ids[0]
	}
}

// GetSteering возвращает значение steering (-1.0 .. +1.0)
func (h *Handler) GetSteering() float64 {
	if !h.cfg.UseGamepad {
		return h.getKeyboardSteering()
	}

	val := h.getGamepadAxis(h.cfg.Axis.Steering)
	if val == 0 && h.cfg.UseKeyboard {
		val = h.getKeyboardSteering()
	}
	return applyDeadzone(val, h.cfg.Deadzone)
}

// GetMove возвращает значение движения (-1.0 .. +1.0)
func (h *Handler) GetMove() float64 {
	if !h.cfg.UseGamepad {
		return h.getKeyboardMove()
	}

	val := h.getGamepadAxis(h.cfg.Axis.Move)
	if h.cfg.InvertMove {
		val = -val
	}
	if val == 0 && h.cfg.UseKeyboard {
		val = h.getKeyboardMove()
	}
	return applyDeadzone(val, h.cfg.Deadzone)
}

// GetPan возвращает значение панорамирования (-1.0 .. +1.0)
func (h *Handler) GetPan() float64 {
	if !h.cfg.UseGamepad {
		return 0
	}
	val := h.getGamepadAxis(h.cfg.Axis.Pan)
	return applyDeadzone(val, h.cfg.Deadzone)
}

// GetTilt возвращает значение наклона (-1.0 .. +1.0)
func (h *Handler) GetTilt() float64 {
	if !h.cfg.UseGamepad {
		return 0
	}
	val := h.getGamepadAxis(h.cfg.Axis.Tilt)
	if h.cfg.InvertTilt {
		val = -val
	}
	return applyDeadzone(val, h.cfg.Deadzone)
}

// IsStopPressed возвращает true, если нажата кнопка STOP
func (h *Handler) IsStopPressed() bool {
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		return true
	}
	if h.cfg.UseGamepad && h.lastGamepad >= 0 {
		return inpututil.IsGamepadButtonJustPressed(h.lastGamepad, ebiten.GamepadButton(h.cfg.Buttons.Stop))
	}
	return false
}

// === Внутренние методы ===

func (h *Handler) getGamepadAxis(axis int) float64 {
	if h.lastGamepad < 0 {
		return 0
	}
	return ebiten.GamepadAxisValue(h.lastGamepad, axis)
}

func (h *Handler) getKeyboardSteering() float64 {
	if ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		return -1
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		return 1
	}
	return 0
}

func (h *Handler) getKeyboardMove() float64 {
	if ebiten.IsKeyPressed(ebiten.KeyUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		return 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
		return -1
	}
	return 0
}

func applyDeadzone(v float64, deadzone float64) float64 {
	if math.Abs(v) < deadzone {
		return 0
	}
	// Нормализуем после deadzone
	sign := 1.0
	if v < 0 {
		sign = -1
	}
	return sign * (math.Abs(v) - deadzone) / (1 - deadzone)
}
