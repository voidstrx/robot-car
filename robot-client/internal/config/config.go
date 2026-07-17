package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	WSLVMName string         `json:"wsl_vm_name"`
	HvSocket  HvSocketConfig `json:"hvsocket"`
	Window    WindowConfig   `json:"window"`
}

type HvSocketConfig struct {
	VideoServiceID   string `json:"video_service_id"`
	CommandServiceID string `json:"command_service_id"`
}

type WindowConfig struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Title  string `json:"title"`
}

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
	if cfg.WSLVMName == "" {
		cfg.WSLVMName = "robot"
	}
	if cfg.HvSocket.VideoServiceID == "" {
		cfg.HvSocket.VideoServiceID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	}
	if cfg.HvSocket.CommandServiceID == "" {
		cfg.HvSocket.CommandServiceID = "b2c3d4e5-f6a7-8901-bcde-f12345678901"
	}

	return &cfg, nil
}
