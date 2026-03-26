package main

import "github.com/gin-gonic/gin"

// -- Captcha Handlers --
// The Java backend used tianai-captcha which is Java-specific.
// In the Go backend, captcha is always reported as disabled.
// Admins can still configure captcha_enabled in config, but
// the generate/verify endpoints return stub success responses.

func HandleCaptchaCheck(c *gin.Context) {
	// Check if captcha is enabled via config
	var cfg ViteConfig
	if err := DB.Where("name = ?", "captcha_enabled").First(&cfg).Error; err != nil {
		Rok(c, 0) // disabled by default
		return
	}
	if cfg.Value == "true" {
		Rok(c, 0) // Return 0 (disabled) even if configured - captcha not implemented in Go backend
		return
	}
	Rok(c, 0)
}

func HandleCaptchaGenerate(c *gin.Context) {
	// Return a stub response - captcha not implemented in Go backend
	c.JSON(200, gin.H{
		"repCode": "0000",
		"repMsg":  "success",
		"success": true,
		"repData": gin.H{
			"type":        "DISABLED",
			"token":       "no-captcha-" + generateSecret(),
			"browserInfo": "",
		},
	})
}

func HandleCaptchaVerify(c *gin.Context) {
	var params map[string]interface{}
	c.ShouldBindJSON(&params)
	id := ""
	if v, ok := params["id"]; ok {
		id = v.(string)
	}
	// Always succeed
	c.JSON(200, gin.H{
		"success": true,
		"repCode": "0000",
		"repMsg":  "success",
		"repData": gin.H{"validToken": id},
	})
}
