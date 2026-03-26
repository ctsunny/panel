package main

import (
	"crypto/rand"
	"encoding/hex"
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
		adminUser := AppConfig.AdminUser
		if adminUser == "" {
			adminUser = "admin_user"
		}
		adminPass := AppConfig.AdminPass
		if adminPass == "" {
			// No password configured; generate a random one to avoid using username as password
			adminPass = generateRandomPassword()
			log.Printf("WARNING: ADMIN_PASS not set, generated a random password for admin user %s", adminUser)
		}
		now := nowMs()
		admin := &User{
			User:          adminUser,
			Pwd:           MD5(adminPass),
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
		log.Printf("Created default admin user: %s", adminUser)
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

// generateRandomPassword creates a cryptographically random 16-character hex password.
func generateRandomPassword() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely; fall back to a static value so the server still starts
		return "changeme_please"
	}
	return hex.EncodeToString(b)
}
