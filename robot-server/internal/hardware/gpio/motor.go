package gpio

import (
	"context"
	"fmt"
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
	if duty < 0 { duty = 0 }
	if duty > 100 { duty = 100 }
	p.duty = duty
}

func (p *SoftPWM) Stop() {
	close(p.stopCh)
	p.wg.Wait()
	p.pin.Low()
}

type motorController struct {
	motors map[string]struct {
		in1 rpio.Pin
		in2 rpio.Pin
		pwm *SoftPWM
	}
	state hardware.RobotState
	mu    sync.Mutex
}

func NewMotorController(motorPins map[string]map[string]int, state hardware.RobotState) hardware.MotorController {
	mc := &motorController{
		motors: make(map[string]struct{in1, in2 rpio.Pin; pwm *SoftPWM}),
		state: state,
	}
	for name, pins := range motorPins {
		in1 := rpio.Pin(pins["in1"])
		in2 := rpio.Pin(pins["in2"])
		en := pins["en"]
		in1.Output(); in2.Output(); in1.Low(); in2.Low()
		pwm := NewSoftPWM(en)
		mc.motors[name] = struct{in1, in2 rpio.Pin; pwm *SoftPWM}{in1: in1, in2: in2, pwm: pwm}
	}
	return mc
}

func (mc *motorController) SetSpeed(ctx context.Context, motor string, speed float64) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	m, ok := mc.motors[motor]
	if !ok {
		return fmt.Errorf("motor %s not found", motor)
	}
	if speed > 1 { speed = 1 }
	if speed < -1 { speed = -1 }
	forward := speed >= 0
	duty := int(speed * 100)
	if duty < 0 { duty = -duty }
	if forward {
		m.in1.High(); m.in2.Low()
	} else {
		m.in1.Low(); m.in2.High()
	}
	m.pwm.SetDuty(duty)
	mc.state.UpdateMotorSpeed(motor, speed)
	fmt.Printf("⚙️ Motor %s: %.2f\n", motor, speed)
	return nil
}

func (mc *motorController) Stop(motor string) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	m, ok := mc.motors[motor]
	if !ok { return fmt.Errorf("motor not found") }
	m.in1.Low(); m.in2.Low(); m.pwm.SetDuty(0)
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
