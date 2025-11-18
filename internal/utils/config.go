package utils

import (
	"os"

	"github.com/ilyakaznacheev/cleanenv"
)

type AppConfig struct {
	App struct {
		Port     string `yaml:"port"`
		LogLevel string `yaml:"log_level"`
	} `yaml:"app"`

	Postgres struct {
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		Dbname   string `yaml:"dbname"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		SSLMode  string `yaml:"ssl_mode"`
	} `yaml:"postgres"`

	Minio struct {
		Endpoint  string `yaml:"endpoint"`
		AccessKey string `yaml:"access_key"`
		SecretKey string `yaml:"secret_key"`
		UseSSL    bool   `yaml:"use_ssl"`
		Bucket    string `yaml:"bucket"`
	} `yaml:"minio"`
}

func LoadConfig(path string) (*AppConfig, error) {
	var cfg AppConfig
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}
	if err := cleanenv.ReadConfig(path, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
