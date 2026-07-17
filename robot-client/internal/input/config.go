package input

// AxisMapping определяет, какой физический axis использовать для каждой функции
type AxisMapping struct {
	Steering int `json:"steering"` // обычно 0 (левая стика X)
	Move     int `json:"move"`     // обычно 1 (левая стика Y)
	Pan      int `json:"pan"`      // обычно 2 (правая стика X)
	Tilt     int `json:"tilt"`     // обычно 3 (правая стика Y)
}

// ButtonMapping — кнопки для специальных действий
type ButtonMapping struct {
	Stop int `json:"stop"` // кнопка для STOP (например, кнопка 0 = A / X)
}

// InputConfig — конфигурация ввода
type InputConfig struct {
	Deadzone     float64       `json:"deadzone"`      // 0.15 — 0.25 обычно
	Axis         AxisMapping   `json:"axis"`
	Buttons      ButtonMapping `json:"buttons"`
	InvertMove   bool          `json:"invert_move"`   // true, если вверх стика = вперёд
	InvertTilt   bool          `json:"invert_tilt"`
	UseGamepad   bool          `json:"use_gamepad"`
	UseKeyboard  bool          `json:"use_keyboard"`
}

// DefaultInputConfig возвращает разумные настройки по умолчанию
func DefaultInputConfig() InputConfig {
	return InputConfig{
		Deadzone: 0.18,
		Axis: AxisMapping{
			Steering: 0, // Left Stick X
			Move:     1, // Left Stick Y
			Pan:      2, // Right Stick X
			Tilt:     3, // Right Stick Y
		},
		Buttons: ButtonMapping{
			Stop: 0, // A / X button
		},
		InvertMove:  true,
		InvertTilt:  true,
		UseGamepad:  true,
		UseKeyboard: true,
	}
}
