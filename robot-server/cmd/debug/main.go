package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"robot-server/internal/hardware"
	"robot-server/internal/hardware/gpio"
	"robot-server/internal/hardware/i2c"
)

type Config struct {
	I2CBus      int                        `json:"i2c_bus"`
	PCA9685Addr int                        `json:"pca9685_addr"`
	Ultrasonic  map[string]float64         `json:"ultrasonic"`
	Servos      map[string]i2c.ServoConfig `json:"servos"`
	Motors      map[string]interface{}     `json:"motors"`
	LEDs        map[string]interface{}     `json:"leds"`
	IMU         map[string]float64         `json:"imu"`
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
	case int64:
		return float64(val)
	default:
		return 0
	}
}

func main() {
	fmt.Println("🤖 Robot Server — Тестовая консоль для Raspberry Pi 3 B+")
	fmt.Println("==================================================")

	// === Загрузка конфига ===
	cfgFile, err := os.ReadFile("configs/config.json")
	if err != nil {
		panic(fmt.Sprintf("Не удалось прочитать configs/config.json: %v", err))
	}

	var cfg Config
	if err := json.Unmarshal(cfgFile, &cfg); err != nil {
		panic(fmt.Sprintf("Ошибка парсинга config.json: %v", err))
	}

	// === Инициализация rpio ===
	if err := gpio.InitGPIO(); err != nil {
		panic(fmt.Sprintf("hardware.InitGPIO: %v", err))
	}
	defer gpio.CloseGPIO()

	state := hardware.NewState()

	// === Ультразвук ===
	trigPin := int(cfg.Ultrasonic["trig_pin"])
	echoPin := int(cfg.Ultrasonic["echo_pin"])
	ultra := gpio.NewUltrasonic(trigPin, echoPin, state)

	// === Сервоприводы PCA9685 ===
	servoCtrl, err := i2c.NewPCA9685ServoController(cfg.I2CBus, cfg.PCA9685Addr, cfg.Servos, state)
	if err != nil {
		fmt.Printf("⚠️  PCA9685 недоступен: %v (продолжаем без серв)\n", err)
		servoCtrl = nil
	} else {
		defer servoCtrl.Close()
	}

	// === Моторы ===
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
	defer motorCtrl.Close()

	// === Светодиоды ===
	var simpleLEDs []map[string]interface{}
	if simpleIface, ok := cfg.LEDs["simple"]; ok {
		if arr, ok := simpleIface.([]interface{}); ok {
			for _, v := range arr {
				if m, ok := v.(map[string]interface{}); ok {
					simpleLEDs = append(simpleLEDs, m)
				}
			}
		}
	}
	wsCfg := make(map[string]interface{})
	if wsIface, ok := cfg.LEDs["ws2811"]; ok {
		if m, ok := wsIface.(map[string]interface{}); ok {
			wsCfg = m
		}
	}
	ledCtrl := gpio.NewLEDController(simpleLEDs, wsCfg, state)
	defer ledCtrl.Close()

	// === Запуск ультразвука ===
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interval := 100
	if iv, ok := cfg.Ultrasonic["interval_ms"]; ok {
		interval = int(iv)
	}
	if err := ultra.Start(ctx, interval); err != nil {
		fmt.Printf("⚠️  Ультразвук не запустился: %v\n", err)
	}

	// === Graceful shutdown ===
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\n\n🛑 Получен сигнал завершения...")
		cancel()
		ultra.Stop()
		if servoCtrl != nil {
			servoCtrl.ReleaseAll()
		}
		motorCtrl.StopAll()
		ledCtrl.Close()
		gpio.CloseGPIO()
		fmt.Println("✅ Всё остановлено. Выход.")
		os.Exit(0)
	}()

	fmt.Println("\n✅ Система инициализирована.")
	fmt.Println("Команды:")
	fmt.Println("  status                    — показать телеметрию")
	fmt.Println("  servo <steering|pan|tilt> < -1.0 ... 1.0 >")
	fmt.Println("  motor <a|b> < -1.0 ... 1.0 >")
	fmt.Println("  motor stop                — остановить оба мотора")
	fmt.Println("  led <id> <on|off>         — простой LED")
	fmt.Println("  ws2811 all <r> <g> <b>    — зажечь все WS2811")
	fmt.Println("  ws2811 render             — применить")
	fmt.Println("  release all               — отпустить все сервы")
	fmt.Println("  exit | quit               — выход")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		cmd := strings.ToLower(parts[0])

		switch cmd {
		case "exit", "quit", "q":
			fmt.Println("Завершение...")
			cancel()
			ultra.Stop()
			if servoCtrl != nil {
				servoCtrl.ReleaseAll()
			}
			motorCtrl.StopAll()
			ledCtrl.Close()
			return

		case "status":
			printStatus(state, ultra, servoCtrl, motorCtrl, ledCtrl)

		case "servo":
			if len(parts) != 3 || servoCtrl == nil {
				fmt.Println("Использование: servo <name> <pos>   (pos от -1.0 до 1.0)")
				continue
			}
			name := parts[1]
			pos, err := strconv.ParseFloat(parts[2], 64)
			if err != nil {
				fmt.Println("Ошибка: позиция должна быть числом")
				continue
			}
			if err := servoCtrl.SetPosition(ctx, name, pos); err != nil {
				fmt.Printf("Ошибка: %v\n", err)
			}

		case "motor":
			if len(parts) < 2 {
				fmt.Println("motor <a|b> <speed> | motor stop")
				continue
			}
			if parts[1] == "stop" {
				motorCtrl.StopAll()
				continue
			}
			if len(parts) != 3 {
				fmt.Println("motor <a|b> < -1.0 ... 1.0 >")
				continue
			}
			motorName := parts[1]
			speed, err := strconv.ParseFloat(parts[2], 64)
			if err != nil {
				fmt.Println("Ошибка скорости")
				continue
			}
			motorCtrl.SetSpeed(ctx, motorName, speed)

		case "led":
			if len(parts) != 3 {
				fmt.Println("led <id> <on|off>")
				continue
			}
			id := parts[1]
			on := parts[2] == "on" || parts[2] == "1"
			ledCtrl.SetSimpleLED(id, on)

		case "ws2811":
			if len(parts) < 2 {
				fmt.Println("ws2811 all <r> <g> <b> | ws2811 render")
				continue
			}
			if parts[1] == "all" && len(parts) == 5 {
				r, _ := strconv.Atoi(parts[2])
				g, _ := strconv.Atoi(parts[3])
				b, _ := strconv.Atoi(parts[4])
				ledCtrl.SetAllWS2811(byte(r), byte(g), byte(b))
				fmt.Println("WS2811 цвета установлены (нужен render)")
			} else if parts[1] == "render" {
				if err := ledCtrl.RenderWS2811(); err != nil {
					fmt.Printf("Render error: %v\n", err)
				} else {
					fmt.Println("WS2811 применено")
				}
			}

		case "release":
			if len(parts) == 2 && parts[1] == "all" && servoCtrl != nil {
				servoCtrl.ReleaseAll()
			} else {
				fmt.Println("release all")
			}

		default:
			fmt.Println("Неизвестная команда. Введите 'status' для справки.")
		}
	}
}

func printStatus(state hardware.RobotState, ultra hardware.Ultrasonic, servo hardware.ServoController, motor hardware.MotorController, led hardware.LEDController) {
	fmt.Println("\n=== 📊 ТЕЛЕМЕТРИЯ РОБОТА ===")
	fmt.Printf("📏 Дистанция: %.1f см\n", ultra.GetDistance())

	if servo != nil {
		fmt.Printf("🔧 Steering: %.2f\n", servo.GetPosition("steering"))
		fmt.Printf("🔧 Pan:      %.2f\n", servo.GetPosition("pan"))
		fmt.Printf("🔧 Tilt:     %.2f\n", servo.GetPosition("tilt"))
	}

	fmt.Printf("⚙️  Motor A:   %.2f\n", motor.GetSpeed("a"))
	fmt.Printf("⚙️  Motor B:   %.2f\n", motor.GetSpeed("b"))

	fmt.Println("💡 LEDs: left_r, left_g, left_b, right_r, right_g, right_b, plata_*")
	fmt.Println("============================\n")
}
