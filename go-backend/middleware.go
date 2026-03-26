package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const adminRoleID = 0

// JWTMiddleware validates JWT token for all /api/** routes (except exclusions)
func JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.JSON(http.StatusUnauthorized, RErrR("未登录或token已过期"))
			c.Abort()
			return
		}

		if !ValidateToken(token) {
			c.JSON(http.StatusUnauthorized, RErrR("无效的token或token已过期"))
			c.Abort()
			return
		}

		userID, _ := GetUserIDFromToken(token)
		roleID, _ := GetRoleIDFromToken(token)
		c.Set("userID", userID)
		c.Set("roleID", roleID)
		c.Next()
	}
}

// RequireAdmin middleware checks admin role (roleID == 0)
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		roleID, ok := c.Get("roleID")
		if !ok || roleID.(int) != adminRoleID {
			Rerr(c, "需要管理员权限")
			c.Abort()
			return
		}
		c.Next()
	}
}

// CORSMiddleware adds permissive CORS headers
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Authorization, subscription-userinfo")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// GetCurrentUserID retrieves the authenticated user's ID from context
func GetCurrentUserID(c *gin.Context) int64 {
	v, _ := c.Get("userID")
	if v == nil {
		return 0
	}
	return v.(int64)
}

// GetCurrentRoleID retrieves the authenticated user's role from context
func GetCurrentRoleID(c *gin.Context) int {
	v, _ := c.Get("roleID")
	if v == nil {
		return -1
	}
	return v.(int)
}

// IsAdmin returns true if the current user is an admin
func IsAdmin(c *gin.Context) bool {
	return GetCurrentRoleID(c) == adminRoleID
}

// PanelGateMiddleware blocks access to the frontend unless the panel access cookie is set.
// The cookie is granted by visiting the panel's secret entry path (/{panelPath}).
// API, flow, WebSocket, and static asset paths are always allowed through.
func PanelGateMiddleware(panelPath string, jwtSecret string) gin.HandlerFunc {
	const cookieName = "_panel"
	// Derive the cookie token using HMAC-SHA256 so it cannot be forged without the JWT secret
	mac := hmac.New(sha256.New, []byte(jwtSecret))
	mac.Write([]byte(panelPath))
	cookieToken := hex.EncodeToString(mac.Sum(nil))
	entryFull := "/" + panelPath
	entryTrail := "/" + panelPath + "/"

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Always allow API, flow, WebSocket, and asset paths
		if strings.HasPrefix(path, "/api/") ||
			strings.HasPrefix(path, "/flow/") ||
			path == "/system-info" ||
			strings.HasPrefix(path, "/assets/") {
			c.Next()
			return
		}

		// The secret entry point: set the access cookie and redirect to "/"
		if path == entryFull || path == entryTrail {
			// secure=false is intentional: the panel runs over plain HTTP; setting secure=true
			// would prevent the cookie from being sent and break the feature entirely. // lgtm[go/cookie-secure-not-set]
			c.SetCookie(cookieName, cookieToken, 86400*30, "/", "", false, true)
			c.Redirect(http.StatusFound, "/")
			c.Abort()
			return
		}

		// For all other frontend paths, require the access cookie
		val, err := c.Cookie(cookieName)
		if err != nil || val != cookieToken {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Next()
	}
}

// GetTokenFromHeader extracts the token from Authorization header
func GetTokenFromHeader(c *gin.Context) string {
	token := c.GetHeader("Authorization")
	token = strings.TrimPrefix(token, "Bearer ")
	return token
}
