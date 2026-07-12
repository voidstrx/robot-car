package gpio

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/stianeikeland/go-rpio/v4"

	"robot-server/internal/hardware"
)

type SoftPWM struct {
	pin    rpio.Pin
	duty   int
	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewSoftPWM(pinNum int) *SoftPWM {
	pin := rpio.Pin(pinNum)
	pin.Output()
	pin.Low()
	spwm := &SoftPWM{pin: pin, duty: 0, stopCh: make(chan struct{})}
	spwm.start()
	return spwm
}

func (p *SoftPWM) start() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-p.stopCh:
				p.pin.Low()
				return
			default:
				if p.duty <= 0 {
					p.pin.Low()
					time.Sleep(8 * time.Millisecond)
					continue
				}
				if p.duty >= 100 {
					p.pin.High()
					time.Sleep(8 * time.Millisecond)
					continue
				}
				period := 8 * time.Millisecond
				onTime := time.Duration(float64(period) * float64(p.duty) / 100)
				offTime := period - onTime
				p.pin.High()
				time.Sleep(onTime)
				p.pin.Low()
				time.Sleep(offTime)
			}
		}
	}()
}

func (p *SoftPWM) SetDuty(duty int) {
	if duty < 0 {
		duty = 0
	}
	if duty > 100 {
		duty = 100
	}
	p.duty = duty
}

func (p *SoftPWM) Stop() {
	close(p.stopCh)
	p.wg.Wait()
	p.pin.Low()
}

// ====================== MOTOR CONTROLLER ======================

type motorController struct {
	motors map[string]struct {
		in1 rpio.Pin
		in2 rpio.Pin
		pwm *SoftPWM
	}
	maxDutyPercent int
	lastSpeed      map[string]float64
	state          hardware.RobotState
	mu             sync.Mutex
}

func NewMotorController(motorPins map[string]map[string]int, maxDuty float64, state hardware.RobotState) hardware.MotorController {
	mc := &motorController{
		motors: make(map[string]struct {
			in1, in2 rpio.Pin
			pwm      *SoftPWM
		}),
		maxDutyPercent: clamp(int(maxDuty), 0, 100),
		lastSpeed:      make(map[string]float64),
		state:          state,
	}

	for name, pins := range motorPins {
		in1 := rpio.Pin(pins["in1"])
		in2 := rpio.Pin(pins["in2"])
		en := pins["en"]

		in1.Output()
		in2.Output()
		in1.Low()
		in2.Low()

		pwm := NewSoftPWM(en)

		mc.motors[name] = struct {
			in1 rpio.Pin
			in2 rpio.Pin
			pwm *SoftPWM
		}{in1: in1, in2: in2, pwm: pwm}
	}
	return mc
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

const rampStep = 0.18 // подбирай: меньше = плавнее

func (mc *motorController) SetSpeed(ctx context.Context, motor string, targetSpeed float64) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	m, ok := mc.motors[motor]
	if !ok {
		return fmt.Errorf("motor %s not found", motor)
	}

	// Ограничение
	if targetSpeed > 1 {
		targetSpeed = 1
	}
	if targetSpeed < -1 {
		targetSpeed = -1
	}

	currentSpeed := mc.lastSpeed[motor]

	// Ramp logic
	if math.Abs(targetSpeed) > math.Abs(currentSpeed) {
		step := math.Copysign(rampStep, targetSpeed)
		currentSpeed += step

		if math.Abs(currentSpeed) > math.Abs(targetSpeed) {
			currentSpeed = targetSpeed
		}
	} else {
		currentSpeed = targetSpeed // торможение сразу
	}

	mc.lastSpeed[motor] = currentSpeed

	// Применяем
	forward := currentSpeed >= 0
	duty := int(math.Abs(currentSpeed) * float64(mc.maxDutyPercent))

	if forward {
		m.in1.High()
		m.in2.Low()
	} else {
		m.in1.Low()
		m.in2.High()
	}

	m.pwm.SetDuty(duty)
	mc.state.UpdateMotorSpeed(motor, currentSpeed)

	fmt.Printf("⚙️ Motor %s: target=%.2f actual=%.2f duty=%d%%\n", motor, targetSpeed, currentSpeed, duty)
	return nil
}

func (mc *motorController) Stop(motor string) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	m, ok := mc.motors[motor]
	if !ok {
		return fmt.Errorf("motor %s not found", motor)
	}

	m.in1.Low()
	m.in2.Low()
	m.pwm.SetDuty(0)
	mc.lastSpeed[motor] = 0
	mc.state.UpdateMotorSpeed(motor, 0)

	fmt.Printf("🛑 Motor %s STOP\n", motor)
	return nil
}

func (mc *motorController) StopAll() error {
	for name := range mc.motors {
		_ = mc.Stop(name)
	}
	return nil
}

func (mc *motorController) GetSpeed(motor string) float64 {
	return mc.state.GetMotorSpeed(motor)
}

func (mc *motorController) Close() error {
	mc.StopAll()
	for _, m := range mc.motors {
		m.pwm.Stop()
	}
	return nil
}
