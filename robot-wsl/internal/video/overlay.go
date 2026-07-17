package video

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"robot-wsl/internal/state"
)

// OverlayConfig — настройки отрисовки (не перекрывает основное видео)
type OverlayConfig struct {
	BarHeight int
	Padding   int
	TextColor color.RGBA
	BgColor   color.RGBA
}

var DefaultOverlay = OverlayConfig{
	BarHeight: 52,
	Padding:   10,
	TextColor: color.RGBA{0, 255, 120, 255}, // ярко-зелёный
	BgColor:   color.RGBA{0, 0, 0, 200},     // почти непрозрачный чёрный
}

// drawString рисует текст на изображении
func drawString(img *image.RGBA, x, y int, label string, col color.RGBA) {
	point := fixed.Point26_6{
		X: fixed.I(x),
		Y: fixed.I(y),
	}
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  point,
	}
	d.DrawString(label)
}

// DrawOverlay рисует верхнюю панель с режимом и телеметрией.
// Панель занимает только верхние ~52 пикселя — основное видео не перекрывается.
func DrawOverlay(frame *image.RGBA, cfg OverlayConfig) {
	if frame == nil {
		return
	}

	bounds := frame.Bounds()
	w := bounds.Dx()

	// Полупрозрачная панель сверху
	barRect := image.Rect(0, 0, w, cfg.BarHeight)
	draw.Draw(frame, barRect, &image.Uniform{cfg.BgColor}, image.Point{}, draw.Over)

	mode := state.Global.GetMode()
	tel := state.Global.GetTelemetry()

	// Левая часть — режим
	modeText := fmt.Sprintf("[%s]", mode)
	drawString(frame, cfg.Padding, cfg.BarHeight-cfg.Padding-2, modeText, cfg.TextColor)

	// Центр/право — телеметрия
	telText := fmt.Sprintf("DIST:%.1fcm  MOT:%.2f/%.2f  ACC:%.1f,%.1f,%.1f  GYRO:%.1f,%.1f,%.1f",
		tel.Distance,
		tel.MotorA, tel.MotorB,
		tel.AccelX, tel.AccelY, tel.AccelZ,
		tel.GyroX, tel.GyroY, tel.GyroZ,
	)
	drawString(frame, 160, cfg.BarHeight-cfg.Padding-2, telText, color.RGBA{200, 200, 200, 255})
}

// DrawOverlayBottom — панель снизу (если верхняя мешает обзору)
func DrawOverlayBottom(frame *image.RGBA, cfg OverlayConfig) {
	if frame == nil {
		return
	}
	bounds := frame.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	barRect := image.Rect(0, h-cfg.BarHeight, w, h)
	draw.Draw(frame, barRect, &image.Uniform{cfg.BgColor}, image.Point{}, draw.Over)

	mode := state.Global.GetMode()
	tel := state.Global.GetTelemetry()

	modeText := fmt.Sprintf("[%s]", mode)
	drawString(frame, cfg.Padding, h-cfg.Padding-2, modeText, cfg.TextColor)

	telText := fmt.Sprintf("DIST:%.1f  M:%.2f/%.2f", tel.Distance, tel.MotorA, tel.MotorB)
	drawString(frame, 160, h-cfg.Padding-2, telText, color.RGBA{200, 200, 200, 255})
}
