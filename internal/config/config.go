package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port   int    `yaml:"port"`
		DBPath string `yaml:"db_path"`
	} `yaml:"server"`
	Auth struct {
		AdminAPIKeys []string `yaml:"admin_api_keys"`
	} `yaml:"auth"`
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
