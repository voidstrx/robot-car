package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	ServerAddr string         `json:"server_addr"`
	RtspURL    string         `json:"rtsp_url"`
	HvSocket   HvSocketConfig `json:"hvsocket"`
	Brain      BrainConfig    `json:"brain"`
}

type HvSocketConfig struct {
	ServiceID string `json:"service_id"`
}

type BrainConfig struct {
	Enabled bool         `json:"enabled"`
	Mode    string       `json:"mode"`
	Vision  VisionConfig `json:"vision"`
}

type VisionConfig struct {
	ModelPath           string  `json:"model_path"`
	ConfidenceThreshold float64 `json:"confidence_threshold"`
}

// LoadConfig загружает конфиг из файла
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Значения по умолчанию
	if cfg.RtspURL == "" {
		cfg.RtspURL = "rtsp://192.168.137.213:8554/robot"
	}
	if cfg.Brain.Vision.ConfidenceThreshold == 0 {
		cfg.Brain.Vision.ConfidenceThreshold = 0.3
	}

	return &cfg, nil
}
