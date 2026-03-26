package main

import (
	"log"
	"os"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/glebarez/sqlite"
)

var DB *gorm.DB

func InitDB() {
	// Ensure data directory exists
	dir := getDirFromPath(AppConfig.DBPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create database directory: %v", err)
		}
	}

	var err error
	DB, err = gorm.Open(sqlite.Open(AppConfig.DBPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Enable WAL mode for better concurrent write performance
	DB.Exec("PRAGMA journal_mode=WAL")
	DB.Exec("PRAGMA synchronous=NORMAL")
	DB.Exec("PRAGMA cache_size=10000")
	DB.Exec("PRAGMA foreign_keys=ON")

	// Auto-migrate schema
	err = DB.AutoMigrate(
		&User{},
		&Node{},
		&Tunnel{},
		&Forward{},
		&UserTunnel{},
		&SpeedLimit{},
		&StatisticsFlow{},
		&ViteConfig{},
	)
	if err != nil {
		log.Fatalf("Failed to auto-migrate database: %v", err)
	}

	// Seed default data
	seedDefaultData()

	log.Println("Database initialized successfully")
}

func seedDefaultData() {
	// Default admin user
	var count int64
	DB.Model(&User{}).Where("role_id = 0").Count(&count)
	if count == 0 {
		now := nowMs()
		// Default password: admin_user (MD5 = 3c85cdebade1c51cf64ca9f3c09d182d)
		admin := &User{
			User:          "admin_user",
			Pwd:           "3c85cdebade1c51cf64ca9f3c09d182d",
			RoleID:        0,
			ExpTime:       2727251700000,
			Flow:          99999,
			InFlow:        0,
			OutFlow:       0,
			FlowResetTime: 1,
			Num:           99999,
			CreatedTime:   now,
			Status:        1,
		}
		DB.Create(admin)
		log.Println("Created default admin user: admin_user / admin_user")
	}

	// Default app_name config
	var cfgCount int64
	DB.Model(&ViteConfig{}).Where("name = ?", "app_name").Count(&cfgCount)
	if cfgCount == 0 {
		DB.Create(&ViteConfig{
			Name:  "app_name",
			Value: "flux",
			Time:  nowMs(),
		})
	}
}

func getDirFromPath(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return ""
}
