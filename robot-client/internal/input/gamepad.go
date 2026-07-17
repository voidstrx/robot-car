package input

import (
	"github.com/hajimehoshi/ebiten/v2"
)

type Gamepad struct {
	id       ebiten.GamepadID
	steering float64
	move     float64
	pan      float64
	tilt     float64
}

func NewGamepad() *Gamepad {
	return &Gamepad{}
}

func (g *Gamepad) Update() {
	g.steering = 0
	g.move = 0
	g.pan = 0
	g.tilt = 0

	ids := ebiten.GamepadIDs()
	if len(ids) == 0 {
		return
	}
	g.id = ids[0]

	g.steering = normalize(ebiten.GamepadAxisValue(g.id, 2))
	g.move = -normalize(ebiten.GamepadAxisValue(g.id, 3))
	g.pan = normalize(ebiten.GamepadAxisValue(g.id, 0))
	g.tilt = -normalize(ebiten.GamepadAxisValue(g.id, 1))
}

func normalize(v float64) float64 {
	if v > -0.15 && v < 0.15 {
		return 0
	}
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

func (g *Gamepad) Steering() float64 { return g.steering }
func (g *Gamepad) Move() float64     { return g.move }
func (g *Gamepad) Pan() float64      { return g.pan }
func (g *Gamepad) Tilt() float64     { return g.tilt }
