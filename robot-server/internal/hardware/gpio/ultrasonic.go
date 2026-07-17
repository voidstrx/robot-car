package gpio

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/stianeikeland/go-rpio/v4"

	"robot-server/internal/hardware"
)

type ultrasonic struct {
	trig    rpio.Pin
	echo    rpio.Pin
	state   hardware.RobotState
	running bool
	mu      sync.Mutex
	stopCh  chan struct{}
}

func NewUltrasonic(trigPin, echoPin int, state hardware.RobotState) hardware.Ultrasonic {
	return &ultrasonic{
		trig:   rpio.Pin(trigPin),
		echo:   rpio.Pin(echoPin),
		state:  state,
		stopCh: make(chan struct{}),
	}
}

func (u *ultrasonic) Start(ctx context.Context, intervalMs int) error {
	u.mu.Lock()
	if u.running {
		u.mu.Unlock()
		return nil
	}
	u.running = true
	u.mu.Unlock()

	u.trig.Output()
	u.echo.Input()
	u.echo.Pull(rpio.PullDown)

	go func() {
		fmt.Printf("📡 Ультразвук запущен (интервал %d мс)\n", intervalMs)
		ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				u.Stop()
				return
			case <-u.stopCh:
				return
			case <-ticker.C:
				dist := u.measureDistance()
				if u.state != nil {
					u.state.UpdateDistance(dist)
				}
				// Вывод только по команде "status" — без спама в консоль
			}
		}
	}()
	return nil
}

func (u *ultrasonic) Stop() {
	u.mu.Lock()
	if !u.running {
		u.mu.Unlock()
		return
	}
	u.running = false
	u.mu.Unlock()
	close(u.stopCh)
	fmt.Println("\n🛑 Ультразвук остановлен")
}

func (u *ultrasonic) GetDistance() float64 {
	if u.state == nil {
		return 0
	}
	return u.state.GetDistance()
}

func (u *ultrasonic) measureDistance() float64 {
	u.trig.Low()
	time.Sleep(2 * time.Microsecond)
	u.trig.High()
	time.Sleep(10 * time.Microsecond)
	u.trig.Low()

	start := time.Now()
	timeout := start.Add(50 * time.Millisecond) // увеличил таймаут

	for u.echo.Read() == rpio.Low {
		if time.Now().After(timeout) {
			return 0
		}
	}
	t1 := time.Now()

	for u.echo.Read() == rpio.High {
		if time.Now().After(timeout) {
			return 0
		}
	}
	t2 := time.Now()

	duration := t2.Sub(t1).Seconds()
	dist := (duration * 34000) / 2

	if dist > 999 || dist < 0 {
		return -1
	}
	return dist
}
