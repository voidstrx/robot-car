package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	grpcserver "robot-server/internal/grpc"
	"robot-server/internal/hardware"
	"robot-server/internal/hardware/gpio"
	"robot-server/internal/hardware/i2c"
)

var mediaMTXCmd *exec.Cmd

func startMediaMTX() {
	log.Println("[MediaMTX] Запуск...")
	mediaMTXCmd = exec.Command("/opt/mediamtx/mediamtx")
	mediaMTXCmd.Dir = "/opt/mediamtx"
	mediaMTXCmd.Stderr = log.Writer()

	if err := mediaMTXCmd.Start(); err != nil {
		log.Printf("[MediaMTX] Ошибка запуска: %v", err)
		return
	}
	log.Println("[MediaMTX] Запущен. RTSP: rtsp://<ip_rpi>:8554/robot")
}

func stopMediaMTX() {
	if mediaMTXCmd != nil && mediaMTXCmd.Process != nil {
		log.Println("[MediaMTX] Остановка...")
		mediaMTXCmd.Process.Signal(syscall.SIGTERM)

		done := make(chan error, 1)
		go func() { done <- mediaMTXCmd.Wait() }()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			mediaMTXCmd.Process.Kill()
		}
	}
}

func main() {
	fmt.Println("Robot Server (gRPC + MediaMTX + MPU-6050)")
	fmt.Println("============================================")

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
		I2CBus      int                        `json:"i2c_bus"`
		PCA9685Addr int                        `json:"pca9685_addr"`
		IMU         map[string]interface{}     `json:"imu"`
	}
	json.Unmarshal(cfgFile, &cfg)

	ultra := gpio.NewUltrasonic(int(cfg.Ultrasonic["trig_pin"]), int(cfg.Ultrasonic["echo_pin"]), state)

	// === Запускаем ультразвуковые измерения ===
	if ultra != nil {
		ctx := context.Background()
		if err := ultra.Start(ctx, 100); err != nil {
			log.Printf("[Ultrasonic] Ошибка запуска: %v", err)
		} else {
			log.Println("[Ultrasonic] Измерения запущены (каждые 100 мс)")
		}
	}

	servoCtrl, _ := i2c.NewPCA9685ServoController(cfg.I2CBus, cfg.PCA9685Addr, cfg.Servos, state)

	// Моторы
	motorPins := make(map[string]map[string]int)
	var maxDutyPercent float64 = 100

	for name, pinsIface := range cfg.Motors {
		if name == "max_duty_percent" {
			if v, ok := pinsIface.(float64); ok {
				maxDutyPercent = v
			}
			continue
		}
		if pinsMap, ok := pinsIface.(map[string]interface{}); ok {
			motorPins[name] = map[string]int{
				"in1": int(getFloat(pinsMap["in1"])),
				"in2": int(getFloat(pinsMap["in2"])),
				"en":  int(getFloat(pinsMap["en"])),
			}
		}
	}

	motorCtrl := gpio.NewMotorController(motorPins, maxDutyPercent, state)

	// === MPU-6050 ===
	var mpu *i2c.MPU6050

	// === IMU Axis Mapping ===
	axisMap := make(map[string]string)
	axisInvert := make(map[string]bool)

	if cfg.IMU != nil {
		// Загружаем маппинг осей
		if m, ok := cfg.IMU["axis_mapping"].(map[string]interface{}); ok {
			for k, v := range m {
				if s, ok := v.(string); ok {
					axisMap[k] = s
				}
			}
		}
		// Загружаем инверсию
		if inv, ok := cfg.IMU["invert"].(map[string]interface{}); ok {
			for k, v := range inv {
				if b, ok := v.(bool); ok {
					axisInvert[k] = b
				}
			}
		}

		// Инициализируем MPU-6050
		if addrFloat, ok := cfg.IMU["mpu6050_addr"].(float64); ok && addrFloat > 0 {
			addr := uint8(addrFloat)
			var err error
			mpu, err = i2c.NewMPU6050(cfg.I2CBus, addr)
			if err != nil {
				log.Printf("[MPU-6050] Не инициализирован: %v", err)
			} else {
				log.Printf("[MPU-6050] Успешно инициализирован (адрес 0x%X)", addr)
			}
		}
	}

	defer func() {
		if mpu != nil {
			mpu.Close()
		}
	}()

	// === MediaMTX ===
	startMediaMTX()
	defer stopMediaMTX()

	// === gRPC сервер (передаём mpu + маппинг) ===
	srv := grpcserver.NewServer(servoCtrl, motorCtrl, ultra, mpu, axisMap, axisInvert)

	// Обработка сигналов
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		stopMediaMTX()
		motorCtrl.StopAll()
		if servoCtrl != nil {
			servoCtrl.ReleaseAll()
		}
		gpio.CloseGPIO()
		os.Exit(0)
	}()

	log.Println("gRPC сервер запущен на :50051")
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
