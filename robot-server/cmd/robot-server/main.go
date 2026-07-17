package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	grpcserver "robot-server/internal/grpc"
	"robot-server/internal/hardware"
	"robot-server/internal/hardware/gpio"
	i2c "robot-server/internal/hardware/i2c"
	"robot-server/internal/state"
)

func getFloat(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

func main() {
	fmt.Println("robot-server (Raspberry Pi) — с hardware из robot-car-main")
	fmt.Println("========================================================")

	// === Загрузка конфига ===
	cfgFile, err := os.ReadFile("configs/config.json")
	if err != nil {
		log.Fatalf("Не удалось прочитать configs/config.json: %v", err)
	}

	var cfg struct {
		GRPCListen    string                 `json:"grpc_listen"`
		I2CBus        int                    `json:"i2c_bus"`
		PCA9685Addr   int                    `json:"pca9685_addr"`
		Ultrasonic    map[string]interface{} `json:"ultrasonic"`
		Servos        map[string]interface{} `json:"servos"`
		Motors        map[string]interface{} `json:"motors"`
		LEDs          map[string]interface{} `json:"leds"`
		IMU           map[string]interface{} `json:"imu"`
	}
	if err := json.Unmarshal(cfgFile, &cfg); err != nil {
		log.Fatalf("Ошибка парсинга config.json: %v", err)
	}
	if cfg.GRPCListen == "" {
		cfg.GRPCListen = ":50051"
	}

	// === Инициализация GPIO ===
	if err := gpio.InitGPIO(); err != nil {
		log.Fatalf("gpio.InitGPIO: %v", err)
	}
	defer gpio.CloseGPIO()

	// === Состояние ===
	modeState := state.New()           // для режима и сообщений (gRPC)
	robotState := state.NewRobotState() // для хранения состояния железа (моторы, сервы, расстояние)
	log.Printf("Режим по умолчанию: MANUAL")

	// === Инициализация hardware контроллеров ===
	var motorCtrl hardware.MotorController
	var servoCtrl hardware.ServoController
	var ultra hardware.Ultrasonic
	var ledCtrl hardware.LEDController
	var mpu *i2c.MPU6050

	log.Println("=== Инициализация hardware ===")

	// Моторы
	motorPins := make(map[string]map[string]int)
	if motorsIface, ok := cfg.Motors["motors"]; ok {
		if m, ok := motorsIface.(map[string]interface{}); ok {
			for name, pinsIface := range m {
				if pinsMap, ok := pinsIface.(map[string]interface{}); ok {
					motorPins[name] = map[string]int{
						"in1": int(getFloat(pinsMap["in1"])),
						"in2": int(getFloat(pinsMap["in2"])),
						"en":  int(getFloat(pinsMap["en"])),
					}
				}
			}
		}
	}
	maxDuty := 75.0
	if md, ok := cfg.Motors["max_duty_percent"].(float64); ok {
		maxDuty = md
	}
	log.Printf("[Hardware] Инициализация моторов (max_duty=%.0f%%)...", maxDuty)
	motorCtrl = gpio.NewMotorController(motorPins, maxDuty, robotState)
	defer motorCtrl.Close()
	log.Println("[Hardware] Моторы инициализированы")

	// Сервоприводы (PCA9685)
	if cfg.PCA9685Addr != 0 && len(cfg.Servos) > 0 {
		// Преобразуем map[string]interface{} в map[string]i2c.ServoConfig
		servoConfigs := make(map[string]i2c.ServoConfig)
		for name, v := range cfg.Servos {
			if m, ok := v.(map[string]interface{}); ok {
				servoConfigs[name] = i2c.ServoConfig{
					Channel:     int(getFloat(m["channel"])),
					MinPulse:    int(getFloat(m["min_pulse"])),
					CenterPulse: int(getFloat(m["center_pulse"])),
					MaxPulse:    int(getFloat(m["max_pulse"])),
					Invert:      m["invert"] == true || m["invert"] == "true",
				}
			}
		}

		if len(servoConfigs) > 0 {
			log.Printf("[Hardware] Инициализация PCA9685 (адрес 0x%02X, %d сервоприводов)...", cfg.PCA9685Addr, len(servoConfigs))
			var err error
			servoCtrl, err = i2c.NewPCA9685ServoController(cfg.I2CBus, cfg.PCA9685Addr, servoConfigs, robotState)
			if err != nil {
				log.Printf("⚠️  PCA9685 недоступен: %v (продолжаем без серв)", err)
				servoCtrl = nil
			} else {
				defer servoCtrl.Close()
				log.Println("[Hardware] PCA9685 инициализирован")
			}
		}
	}

	// Ультразвук
	if trig, ok := cfg.Ultrasonic["trig_pin"].(float64); ok {
		echo := int(cfg.Ultrasonic["echo_pin"].(float64))
		ultra = gpio.NewUltrasonic(int(trig), echo, robotState)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		interval := 100
		if iv, ok := cfg.Ultrasonic["interval_ms"].(float64); ok {
			interval = int(iv)
		}
		if err := ultra.Start(ctx, interval); err != nil {
			log.Printf("⚠️  Ультразвук не запустился: %v", err)
		} else {
			log.Println("[Hardware] Ультразвук запущен")
		}
	}

	// LED (простые + WS2811)
	// Для простоты — создаём с пустыми конфигами если секция отсутствует
	ledCtrl = gpio.NewLEDController(nil, nil, nil)
	defer ledCtrl.Close()

	// MPU6050
	if imuCfg, ok := cfg.IMU["mpu6050_addr"].(float64); ok && imuCfg != 0 {
		var err error
		mpu, err = i2c.NewMPU6050(cfg.I2CBus, uint8(imuCfg))
		if err != nil {
			log.Printf("⚠️  MPU6050 недоступен: %v", err)
			mpu = nil
		}
	}

	// === gRPC сервер ===
	log.Println("=== Hardware инициализация завершена ===")
	srv := grpcserver.NewServer(modeState, motorCtrl, servoCtrl, ultra, ledCtrl, mpu)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Получен сигнал завершения...")
		if ultra != nil {
			ultra.Stop()
		}
		if servoCtrl != nil {
			servoCtrl.ReleaseAll()
		}
		if motorCtrl != nil {
			motorCtrl.StopAll()
		}
		if ledCtrl != nil {
			ledCtrl.Close()
		}
		gpio.CloseGPIO()
		os.Exit(0)
	}()

	log.Printf("gRPC сервер запускается на %s", cfg.GRPCListen)
	if err := grpcserver.StartGRPCServer(cfg.GRPCListen, srv); err != nil {
		log.Fatalf("gRPC server error: %v", err)
	}
}
