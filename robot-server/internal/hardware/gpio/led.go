package gpio

import (
	"fmt"
	"sync"

	ws2811 "github.com/rpi-ws281x/rpi-ws281x-go"
	"github.com/stianeikeland/go-rpio/v4"

	"robot-server/internal/hardware"
)

// helper to safely get int from json unmarshaled interface{} (float64)
func getInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case int64:
		return int(val)
	default:
		return 0
	}
}

func getBool(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

type simpleLED struct {
	pin       rpio.Pin
	activeLow bool
}

type ledController struct {
	simpleLEDs map[string]*simpleLED
	ws2811Dev  *ws2811.WS2811
	wsEnabled  bool
	ledCount   int
	state      hardware.RobotState
	mu         sync.Mutex
	closed     bool
}

func NewLEDController(simpleLEDsCfg []map[string]interface{}, wsCfg map[string]interface{}, state hardware.RobotState) hardware.LEDController {
	lc := &ledController{
		simpleLEDs: make(map[string]*simpleLED),
		state:      state,
	}

	// Simple GPIO LEDs
	for _, ledCfg := range simpleLEDsCfg {
		id := ""
		if v, ok := ledCfg["id"]; ok {
			id = fmt.Sprintf("%v", v)
		}
		pin := getInt(ledCfg["pin"])
		activeLow := getBool(ledCfg["active_low"])

		if pin == 0 || id == "" {
			continue
		}

		p := rpio.Pin(pin)
		p.Output()
		if activeLow {
			p.High()
		} else {
			p.Low()
		}

		lc.simpleLEDs[id] = &simpleLED{pin: p, activeLow: activeLow}
		state.UpdateSimpleLED(id, false)
	}

	// WS2811
	if enabled := getBool(wsCfg["enabled"]); enabled {
		opt := ws2811.DefaultOptions
		opt.Channels[0].Brightness = int(getInt(wsCfg["brightness"]))
		opt.Channels[0].LedCount = getInt(wsCfg["led_count"])
		opt.Channels[0].GpioPin = getInt(wsCfg["gpio_pin"])

		dev, err := ws2811.MakeWS2811(&opt)
		if err == nil {
			if err := dev.Init(); err == nil {
				lc.ws2811Dev = dev
				lc.wsEnabled = true
				lc.ledCount = opt.Channels[0].LedCount
				fmt.Println("✅ WS2811 инициализирован")
			} else {
				fmt.Printf("⚠️  WS2811 Init error: %v\n", err)
			}
		} else {
			fmt.Printf("⚠️  WS2811 Make error: %v\n", err)
		}
	}

	return lc
}

func (lc *ledController) SetSimpleLED(id string, on bool) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	led, ok := lc.simpleLEDs[id]
	if !ok {
		return fmt.Errorf("LED %s not found", id)
	}

	if led.activeLow {
		if on {
			led.pin.Low()
		} else {
			led.pin.High()
		}
	} else {
		if on {
			led.pin.High()
		} else {
			led.pin.Low()
		}
	}

	lc.state.UpdateSimpleLED(id, on)
	fmt.Printf("💡 LED %s: %s\n", id, map[bool]string{true: "ON", false: "OFF"}[on])
	return nil
}

func (lc *ledController) SetAllSimpleLEDs(on bool) error {
	for id := range lc.simpleLEDs {
		_ = lc.SetSimpleLED(id, on)
	}
	return nil
}

func (lc *ledController) SetWS2811Pixel(index int, r, g, b byte) error {
	if !lc.wsEnabled || lc.ws2811Dev == nil {
		return fmt.Errorf("WS2811 not enabled")
	}
	if index < 0 || index >= lc.ledCount {
		return fmt.Errorf("invalid WS2811 index")
	}
	lc.ws2811Dev.Leds(0)[index] = uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	return nil
}

func (lc *ledController) SetAllWS2811(r, g, b byte) error {
	if !lc.wsEnabled || lc.ws2811Dev == nil {
		return fmt.Errorf("WS2811 not enabled")
	}
	color := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	for i := 0; i < lc.ledCount; i++ {
		lc.ws2811Dev.Leds(0)[i] = color
	}
	return nil
}

func (lc *ledController) RenderWS2811() error {
	if !lc.wsEnabled || lc.ws2811Dev == nil {
		return fmt.Errorf("WS2811 not enabled")
	}
	return lc.ws2811Dev.Render()
}

func (lc *ledController) Close() error {
	lc.mu.Lock()
	if lc.closed {
		lc.mu.Unlock()
		return nil
	}
	lc.closed = true
	lc.mu.Unlock()

	// Turn off all LEDs (safe to call even if already off)
	lc.SetAllSimpleLEDs(false)

	if lc.wsEnabled && lc.ws2811Dev != nil {
		_ = lc.SetAllWS2811(0, 0, 0)
		_ = lc.RenderWS2811()
		lc.ws2811Dev.Fini()
		lc.ws2811Dev = nil
		lc.wsEnabled = false
	}
	return nil
}
