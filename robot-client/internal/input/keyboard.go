package input

import (
	"github.com/hajimehoshi/ebiten/v2"
)

type Keyboard struct {
	steering float64
	move     float64
	pan      float64
	tilt     float64
}

func NewKeyboard() *Keyboard {
	return &Keyboard{}
}

func (k *Keyboard) Update() {
	k.steering = 0
	k.move = 0
	k.pan = 0
	k.tilt = 0

	if ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		k.steering = -1
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		k.steering = 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		k.move = 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
		k.move = -1
	}
	if ebiten.IsKeyPressed(ebiten.KeyQ) {
		k.pan = -1
	}
	if ebiten.IsKeyPressed(ebiten.KeyE) {
		k.pan = 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyZ) {
		k.tilt = -1
	}
	if ebiten.IsKeyPressed(ebiten.KeyX) {
		k.tilt = 1
	}
}

func (k *Keyboard) Steering() float64 { return k.steering }
func (k *Keyboard) Move() float64     { return k.move }
func (k *Keyboard) Pan() float64      { return k.pan }
func (k *Keyboard) Tilt() float64     { return k.tilt }
