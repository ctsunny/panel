package main

import (
	"context"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	LoadConfig()
	InitDB()
	StartScheduledTasks()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(CORSMiddleware())
	if AppConfig.PanelPath != "" {
		r.Use(PanelGateMiddleware(AppConfig.PanelPath, AppConfig.JWTSecret))
		log.Println("Panel access path restriction enabled")
	}

	// --- Flow endpoints (no auth, accessed by GOST nodes) ---
	r.Any("/flow/upload", HandleFlowUpload)
	r.Any("/flow/config", HandleFlowConfig)
	r.Any("/flow/test", HandleFlowTest)

	// --- WebSocket (GOST nodes + admin browsers) ---
	r.GET("/system-info", HandleWebSocket)

	// --- Public API ---
	pub := r.Group("/api/v1")
	pub.POST("/user/login", HandleUserLogin)
	pub.POST("/captcha/check", HandleCaptchaCheck)
	pub.POST("/captcha/generate", HandleCaptchaGenerate)
	pub.POST("/captcha/verify", HandleCaptchaVerify)
	pub.POST("/config/get", HandleConfigGet)

	// OpenAPI (no JWT required)
	pub.GET("/open_api/sub_store", HandleOpenAPISubStore)

	// --- Protected API (JWT required) ---
	auth := r.Group("/api/v1")
	auth.Use(JWTMiddleware())

	// User
	auth.POST("/user/package", HandleUserPackage)
	auth.POST("/user/updatePassword", HandleUpdatePassword)
	auth.POST("/config/list", HandleConfigList)

	// Forward (user and admin)
	auth.POST("/forward/create", HandleForwardCreate)
	auth.POST("/forward/list", HandleForwardList)
	auth.POST("/forward/update", HandleForwardUpdate)
	auth.POST("/forward/delete", HandleForwardDelete)
	auth.POST("/forward/force-delete", HandleForwardForceDelete)
	auth.POST("/forward/pause", HandleForwardPause)
	auth.POST("/forward/resume", HandleForwardResume)
	auth.POST("/forward/diagnose", HandleForwardDiagnose)
	auth.POST("/forward/update-order", HandleForwardUpdateOrder)

	// Tunnel (user - get accessible tunnels)
	auth.POST("/tunnel/user/tunnel", HandleTunnelUserTunnel)

	// --- Admin-only API ---
	admin := r.Group("/api/v1")
	admin.Use(JWTMiddleware())
	admin.Use(RequireAdmin())

	// User management
	admin.POST("/user/create", HandleUserCreate)
	admin.POST("/user/list", HandleUserList)
	admin.POST("/user/update", HandleUserUpdate)
	admin.POST("/user/delete", HandleUserDelete)
	admin.POST("/user/reset", HandleResetFlow)

	// Node management
	admin.POST("/node/create", HandleNodeCreate)
	admin.POST("/node/list", HandleNodeList)
	admin.POST("/node/update", HandleNodeUpdate)
	admin.POST("/node/delete", HandleNodeDelete)
	admin.POST("/node/install", HandleNodeInstall)

	// Tunnel management
	admin.POST("/tunnel/create", HandleTunnelCreate)
	admin.POST("/tunnel/list", HandleTunnelList)
	admin.POST("/tunnel/update", HandleTunnelUpdate)
	admin.POST("/tunnel/delete", HandleTunnelDelete)
	admin.POST("/tunnel/diagnose", HandleTunnelDiagnose)
	admin.POST("/tunnel/user/assign", HandleUserTunnelAssign)
	admin.POST("/tunnel/user/list", HandleUserTunnelList)
	admin.POST("/tunnel/user/remove", HandleUserTunnelRemove)
	admin.POST("/tunnel/user/update", HandleUserTunnelUpdate)

	// Speed limit
	admin.POST("/speed-limit/create", HandleSpeedLimitCreate)
	admin.POST("/speed-limit/list", HandleSpeedLimitList)
	admin.POST("/speed-limit/update", HandleSpeedLimitUpdate)
	admin.POST("/speed-limit/delete", HandleSpeedLimitDelete)
	admin.POST("/speed-limit/tunnels", HandleTunnelList) // same as tunnel list

	// Config management
	admin.POST("/config/update", HandleConfigUpdate)
	admin.POST("/config/update-single", HandleConfigUpdateSingle)

	// --- Static file serving (frontend SPA) ---
	setupStaticFiles(r)

	// --- Start server ---
	port := AppConfig.Port
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Panel server starting on port %s", port)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Server stopped")
}

func setupStaticFiles(r *gin.Engine) {
	staticDir := AppConfig.StaticDir

	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		log.Printf("Static files directory not found: %s (frontend not served)", staticDir)
		return
	}

	log.Printf("Serving frontend from: %s", staticDir)

	// Register MIME types
	mime.AddExtensionType(".js", "application/javascript")
	mime.AddExtensionType(".mjs", "application/javascript")
	mime.AddExtensionType(".css", "text/css")
	mime.AddExtensionType(".woff2", "font/woff2")
	mime.AddExtensionType(".woff", "font/woff")

	// Serve all static assets
	r.StaticFS("/assets", http.Dir(filepath.Join(staticDir, "assets")))

	// Serve favicon and other root files
	serveRootFiles(r, staticDir)

	// SPA fallback: all unmatched routes return index.html
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// Don't serve index.html for API routes
		if strings.HasPrefix(path, "/api/") ||
			strings.HasPrefix(path, "/flow/") ||
			path == "/system-info" {
			c.JSON(404, gin.H{"code": -1, "msg": "not found"})
			return
		}

		indexPath := filepath.Join(staticDir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			c.File(indexPath)
		} else {
			c.JSON(404, gin.H{"code": -1, "msg": "not found"})
		}
	})
}

func serveRootFiles(r *gin.Engine, staticDir string) {
	entries, err := os.ReadDir(staticDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "index.html" {
			continue // handled by NoRoute
		}
		filePath := filepath.Join(staticDir, name)
		r.GET("/"+name, func(path string) gin.HandlerFunc {
			return func(c *gin.Context) {
				c.File(path)
			}
		}(filePath))
	}

	// Walk subdirectories except 'assets' (already served above)
	_ = filepath.WalkDir(staticDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(staticDir, path)
		rel = "/" + filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "/assets/") {
			return nil // already served
		}
		if rel == "/index.html" {
			return nil
		}
		p := path
		r.GET(rel, func(c *gin.Context) {
			c.File(p)
		})
		return nil
	})
}
