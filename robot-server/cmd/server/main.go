package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	grpcserver "robot-server/internal/grpc"
	"robot-server/internal/hardware"
	"robot-server/internal/hardware/gpio"
	"robot-server/internal/hardware/i2c"
)

func main() {
	fmt.Println("🤖 Robot Server (gRPC) — Raspberry Pi 3 B+")
	fmt.Println("==================================================")

	if err := gpio.InitGPIO(); err != nil {
		panic(err)
	}
	defer gpio.CloseGPIO()

	state := hardware.NewState()

	cfgFile, _ := os.ReadFile("configs/config.json")
	var cfg struct {
		Ultrasonic  map[string]float64         `json:"ultrasonic"`
		Servos      map[string]i2c.ServoConfig `json:"servos"`
		Motors      map[string]interface{}     `json:"motors"`
		LEDs        map[string]interface{}     `json:"leds"`
		I2CBus      int                        `json:"i2c_bus"`
		PCA9685Addr int                        `json:"pca9685_addr"`
	}
	json.Unmarshal(cfgFile, &cfg)

	ultra := gpio.NewUltrasonic(int(cfg.Ultrasonic["trig_pin"]), int(cfg.Ultrasonic["echo_pin"]), state)
	servoCtrl, _ := i2c.NewPCA9685ServoController(cfg.I2CBus, cfg.PCA9685Addr, cfg.Servos, state)
	motorPins := make(map[string]map[string]int)
	for name, pinsIface := range cfg.Motors {
		if pinsMap, ok := pinsIface.(map[string]interface{}); ok {
			motorPins[name] = map[string]int{
				"in1": int(getFloat(pinsMap["in1"])),
				"in2": int(getFloat(pinsMap["in2"])),
				"en":  int(getFloat(pinsMap["en"])),
			}
		}
	}
	motorCtrl := gpio.NewMotorController(motorPins, state)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ultra.Start(ctx, 100)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
		ultra.Stop()
		if servoCtrl != nil {
			servoCtrl.ReleaseAll()
		}
		motorCtrl.StopAll()
		gpio.CloseGPIO()
		os.Exit(0)
	}()

	srv := grpcserver.NewServer(servoCtrl, motorCtrl, ultra)
	fmt.Println("✅ Сервер готов. Ожидание подключений на :50051")
	if err := grpcserver.StartGRPCServer(":50051", srv); err != nil {
		panic(err)
	}
}

func getFloat(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	default:
		return 0
	}
}
