# robot-car

Full AI-generated for Raspberry PI

robot-client - windows

robot-wsl - ubuntu

robot-server - raspbery pi 3b+

Этап 1: Стабилизация текущей инфраструктуры (Сейчас)
Прежде чем добавлять сложную логику, нужно сделать базовую связку надёжной:

 Перевести обмен между Windows и WSL на Hyper-V Sockets / vsock (ты уже начал).
 Сделать отдельные каналы: видео и команды.
 Реализовать гибридный режим (Manual / Auto) с хранением флага на Raspberry Pi.
 WSL должен каждые 100 мс запрашивать текущий режим у Raspberry Pi и обновлять SharedState.
 WSL должен отрисовывать текущий режим на кадре (MANUAL / AUTO).
 Настроить стабильную передачу кадров (разрешение, качество, FPS).

Цель этапа: У тебя должна быть стабильная связка Windows ↔ WSL ↔ Raspberry Pi с работающим переключением режимов.




Этап 2: Vision Agent (Высокий приоритет)
Это основа всей системы.

YOLO inference на WSL (ONNX + GPU через DirectML / CUDA).
 Постобработка детекций (фильтрация, NMS, трекинг).
Object Tracking (простой трекер или ByteTrack / StrongSORT).
Scene Understanding (базовый):
Определение зоны (рабочая зона, опасная зона и т.д.).
Подсчёт объектов определённых классов.
Простая логика "что происходит в кадре".


Цель этапа: WSL стабильно детектит объекты и отдаёт структурированные данные ([]DetectedObject).

Этап 3: Execution Agent

 Создать структуру AICommand / Command (с полями Steering, Move, Pan, Tilt, Action и т.д.).
 Реализовать отправку команд на Raspberry Pi по gRPC (уже частично есть).
 Добавить защиту (например, не отправлять команды чаще определённой частоты).
 Реализовать два пути отправки команд:
В Manual режиме — напрямую с Windows.
В Auto режиме — из WSL (через Orchestrator).



Этап 4: Orchestrator (Главный контроллер)
Это мозг системы.

 Создать структуру Orchestrator.
 Подключить Vision (получение детекций).
 Подключить Execution Agent (отправка команд).
 Подключить SharedState (чтение данных с робота).
 Реализовать базовый цикл:
Получить детекции.
Получить данные с робота.
Принять решение.
Отправить команду.

 Добавить поддержку режимов (Manual / Auto).


Этап 5: Memory Agent

Краткосрочная память (в RAM) — последние N детекций, последние команды, состояние сцены.
Долгосрочная память — PostgreSQL.
 Таблицы для хранения:
Детекций
Выполненных задач
Событий

 Retrieval (поиск по памяти) — простой semantic search или keyword search.


Этап 6: Planning / Task Agent

Task Generator:
Правила (если увидел человека → приоритет высок)
LLM (генерация задач на основе описания сцены)

 Система приоритизации задач.
 Простая очередь задач.
 Принятие решений (какую задачу выполнять сейчас).


Этап 7: Voice Agent (опционально, но полезно)

 Получение аудио с Raspberry Pi.
 Интеграция Whisper (локально или через API).
 Преобразование речи в команды / текст.
 Отправка команд в Orchestrator.


Этап 8: Финальная интеграция и полировка

 Полная интеграция всех агентов через Orchestrator.
 Система состояний и переходов между режимами.
 Логирование + мониторинг.
 Обработка ошибок и восстановление после падений.
 Тестирование всей цепочки (Manual → Auto → Manual).
 Оптимизация задержки (особенно в Manual режиме).



 Вот системное техническое задание (ТЗ) и описание архитектуры проекта, сформулированное на универсальном инженерном языке. Ты можешь скопировать этот текст и напрямую скормить его другой нейросети (например, для генерации кода, Protobuf-контрактов или обучения моделей).
------------------------------
## System Architecture & Technical Specification: Autonomous Robot Car## 1. Executive Summary
Project Goal: Build a highly adaptive, autonomous differential-drive robot car using a hybrid control architecture.
Hardware Stack: Raspberry Pi 3B+ (On-board Controller), PCA9685 (I2C PWM Driver), L298N/TB6612FNG (DC Motor Driver), MPU6050 (IMU via I2C), HC-SR04 (Ultrasonic Sonar mounted on a 2-DOF Pan/Tilt servo mount), 2x IR Wheel Encoders (Digital Inputs), and a CSI/USB Camera.
Software Stack: Go (Golang) for low-level execution and gRPC server, TensorFlow Lite (TFLite) for Edge AI embedded in Go, and Python/PyTorch/OpenCV for high-level Vision AI and HUD telemetry on a remote PC.
------------------------------
## 2. Hybrid Control Architecture (The 3-Layer Model)## Layer 1: Physical Execution & Safety (Low-Level / Go Code)

* Platform: Runs natively on Raspberry Pi 3B+ in Go. No AI here. High frequency: 100–200 Hz.
* Responsibility:
* Odometry & Kinematics: Track digital pulses from left/right IR encoders to compute linear velocity (v) and distance.
   * State Estimation: Read MPU6050 Z-axis (Yaw rate) and apply a Complementary Filter with encoder data to mitigate gyro drift and wheel slippage.
   * Signal Filtering: Apply a median filter (window size=5) to HC-SR04 distance readings to eliminate ultrasonic spike noise.
   * Actuation: Execute closed-loop PID control for DC motors (Speed) and steering servo (Heading tracking).
   * Hardware Failsafe: High-priority intercept logic. If the filtered ultrasonic distance drops below 15 cm, or if the gRPC connection drops for >100 ms, the Go layer immediately overrides upper-level commands and fires EmergencyBrake().

## Layer 2: Tactical Navigation (Mid-Level / Edge AI)

* Platform: Embedded TensorFlow Lite (int8 quantized) model running inside the Go runtime via CGO. Frequency: 30 Hz.
* Operating Modes:
* Phase A (Training/Calibration): The robot tracks a physical black line on the floor using a multi-channel IR line-follower array. The camera captures frames synchronized with the current PID steering angles and velocities, saving a behavioral cloning dataset.
   * Phase B (Vision-Only Inference): The physical line is removed. The camera frames are downsampled to 96×96 (grayscale). The TFLite CNN model processes the frame combined with current v and ω to output reactive target vectors: Target_Linear_Velocity and Target_Angular_Velocity.
   * Phase C (Blind Navigation - Alternative Mode): If the camera is disabled, a tiny MLP TFLite network processes a spatial distance vector (e.g., 5-point sweep from the panning sonar) to output reactive wall-following or obstacle-avoidance vectors.

## Layer 3: Strategic Vision (High-Level / Remote Big AI)

* Platform: Remote Desktop/Laptop with powerful GPU running Python/OpenCV/YOLO. Frequency: 10–15 Hz.
* Network: Bidirectional gRPC Streaming. The robot streams raw video frames and sensor metadata (including active Pan and Tilt angles of the camera mount) to the PC.
* Responsibility:
* Object Detection: Runs heavy object detection (YOLO) to recognize traffic signs (Stop, Speed limits), traffic lights, and dynamic obstacles.
   * Inverse Perspective Mapping (IPM): Translates 2D bounding boxes into 3D world coordinates relative to the robot's base by calculating the dynamic camera transformation matrix using the streamed Pan (Horizontal) and Tilt (Vertical) servo angles.
   * Command Output: Does not control micro-steering pulses. It acts as a "Navigator", returning high-level semantic constraints to the Go server via gRPC (e.g., SetSpeedLimit(30%), TriggerObjectFollowingMode()).

------------------------------
## 3. Dynamic Camera & Sonar Payload Logic

* Hardware Setup: The camera and HC-SR04 sonar are mounted together on a 2-axis servo bracket controlled by PCA9685 channels (Channel 0: Pan, Channel 15: Tilt).
* Behavioral Sweeping: On straight track segments, the Go server commands the Pan servo to actively sweep ±45°, generating a dynamic situational distance grid.
* Predictive Cornering: During high-speed curves, the Go server automatically slaved the Pan servo to the active Steering angle, forcing the camera and sonar to "look into the turn" before the vehicle chassis rotates.
* Dynamic Pitch (Tilt Control): Slaved to vehicle velocity. At max acceleration, the Tilt servo pitches up to expand the remote AI's visual horizon. During deceleration or tight maneuvers, it pitches down to feed high-resolution ground texture to the Edge AI model.

------------------------------
## 4. Telemetry and HUD Visualization

* HUD Rendering: Executed entirely on the remote PC using OpenCV to offload the Raspberry Pi CPU.
* Data Aggregation: gRPC packets map raw imagery with localized metadata (Encoder ticks, Filtered Gyro Yaw, Sonar Distance, Pan/Tilt angles).
* Visual Output: Renders an augmented reality display for debugging:
1. A bounding box overlaid on signs/obstacles.
   2. A 3D perspective grid mapping the robot's calculated future path based on Layer 2 and Layer 3 outputs.
   3. A virtual radar widget showing the active real-time pointing vector of the physical scanning sensor head.

------------------------------



