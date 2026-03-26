package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// -- ViteConfig Handlers --

func HandleConfigList(c *gin.Context) {
	var configs []ViteConfig
	DB.Find(&configs)
	result := make(map[string]string)
	for _, cfg := range configs {
		result[cfg.Name] = cfg.Value
	}
	Rok(c, result)
}

func HandleConfigGet(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	name, ok := params["name"].(string)
	if !ok || name == "" {
		Rerr(c, "配置名称不能为空")
		return
	}

	var cfg ViteConfig
	if err := DB.Where("name = ?", name).First(&cfg).Error; err != nil {
		Rerr(c, "配置不存在")
		return
	}
	Rok(c, cfg)
}

func HandleConfigUpdate(c *gin.Context) {
	var configMap map[string]string
	if err := c.ShouldBindJSON(&configMap); err != nil {
		Rerr(c, "参数错误")
		return
	}

	for name, value := range configMap {
		if name == "" {
			continue
		}
		upsertConfig(name, value)
	}
	RokMsg(c)
}

func HandleConfigUpdateSingle(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	name, _ := params["name"].(string)
	value, _ := params["value"].(string)
	if name == "" {
		Rerr(c, "配置名称不能为空")
		return
	}

	upsertConfig(name, value)
	RokMsg(c)
}

func upsertConfig(name, value string) {
	var cfg ViteConfig
	if err := DB.Where("name = ?", name).First(&cfg).Error; err != nil {
		// Create
		DB.Create(&ViteConfig{Name: name, Value: value, Time: nowMs()})
	} else {
		// Update
		cfg.Value = value
		cfg.Time = nowMs()
		DB.Save(&cfg)
	}
}

// -- OpenAPI Handler --

func HandleOpenAPISubStore(c *gin.Context) {
	user := c.Query("user")
	pwd := c.Query("pwd")
	tunnel := c.Query("tunnel")
	if tunnel == "" {
		tunnel = "-1"
	}

	if user == "" {
		Rerr(c, "用户不能为空")
		return
	}
	if pwd == "" {
		Rerr(c, "密码不能为空")
		return
	}

	var userInfo User
	if err := DB.Where("user = ?", user).First(&userInfo).Error; err != nil {
		Rerr(c, "鉴权失败")
		return
	}

	if MD5(pwd) != userInfo.Pwd {
		Rerr(c, "鉴权失败")
		return
	}

	const GIGA = 1024 * 1024 * 1024

	var headerValue string
	if tunnel == "-1" {
		headerValue = buildSubHeader(userInfo.OutFlow, userInfo.InFlow, userInfo.Flow*GIGA, userInfo.ExpTime/1000)
	} else {
		var ut UserTunnel
		if err := DB.First(&ut, tunnel).Error; err != nil {
			Rerr(c, "隧道不存在")
			return
		}
		if ut.UserID != userInfo.ID {
			Rerr(c, "隧道不存在")
			return
		}
		headerValue = buildSubHeader(ut.OutFlow, ut.InFlow, ut.Flow*GIGA, ut.ExpTime/1000)
	}

	c.Header("subscription-userinfo", headerValue)
	c.String(200, headerValue)
}

func buildSubHeader(upload, download, total, expire int64) string {
	return buildSubHeaderFormatted(download, upload, total, expire)
}

func buildSubHeaderFormatted(upload, download, total, expire int64) string {
	return fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", download, upload, total, expire)
}
