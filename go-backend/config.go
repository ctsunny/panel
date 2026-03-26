package main

import (
	"log"
	"os"
)

type Config struct {
	JWTSecret string
	DBPath    string
	Port      string
	LogDir    string
	StaticDir string
}

var AppConfig Config

func LoadConfig() {
	AppConfig = Config{
		JWTSecret: getEnv("JWT_SECRET", "default_jwt_secret_please_change"),
		DBPath:    getEnv("DB_PATH", "/data/gost.db"),
		Port:      getEnv("PORT", "6365"),
		LogDir:    getEnv("LOG_DIR", "/data/logs"),
		StaticDir: getEnv("STATIC_DIR", "./static"),
	}

	if AppConfig.JWTSecret == "default_jwt_secret_please_change" {
		log.Println("WARNING: Using default JWT_SECRET. Please set a secure JWT_SECRET environment variable.")
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
