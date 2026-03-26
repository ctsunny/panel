package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// -- SpeedLimit Handlers --

func HandleSpeedLimitCreate(c *gin.Context) {
	var dto CreateSpeedLimitDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	// Validate tunnel
	var tunnel Tunnel
	if err := DB.First(&tunnel, dto.TunnelID).Error; err != nil {
		Rerr(c, "指定的隧道不存在")
		return
	}
	if tunnel.Name != dto.TunnelName {
		Rerr(c, "隧道名称与隧道ID不匹配")
		return
	}

	now := nowMs()
	sl := &SpeedLimit{
		Name:        dto.Name,
		Speed:       dto.Speed,
		TunnelID:    dto.TunnelID,
		TunnelName:  dto.TunnelName,
		CreatedTime: now,
		Status:      1,
	}
	if err := DB.Create(sl).Error; err != nil {
		Rerr(c, "限速规则创建失败")
		return
	}

	// Add limiter in GOST
	speed := fmt.Sprintf("%.1f", float64(sl.Speed)/8.0)
	result := GostAddLimiters(tunnel.InNodeID, sl.ID, speed)
	if !IsOK(result) {
		DB.Delete(&SpeedLimit{}, sl.ID)
		Rerr(c, result.Msg)
		return
	}

	RokMsg(c)
}

func HandleSpeedLimitList(c *gin.Context) {
	var speedLimits []SpeedLimit
	DB.Find(&speedLimits)
	Rok(c, speedLimits)
}

func HandleSpeedLimitUpdate(c *gin.Context) {
	var dto UpdateSpeedLimitDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	var sl SpeedLimit
	if err := DB.First(&sl, dto.ID).Error; err != nil {
		Rerr(c, "限速规则不存在")
		return
	}

	var tunnel Tunnel
	if err := DB.First(&tunnel, dto.TunnelID).Error; err != nil {
		Rerr(c, "指定的隧道不存在")
		return
	}
	if tunnel.Name != dto.TunnelName {
		Rerr(c, "隧道名称与隧道ID不匹配")
		return
	}

	if dto.Name != "" {
		sl.Name = dto.Name
	}
	if dto.Speed != 0 {
		sl.Speed = dto.Speed
	}
	if dto.TunnelID != 0 {
		sl.TunnelID = dto.TunnelID
		sl.TunnelName = dto.TunnelName
	}

	speed := fmt.Sprintf("%.1f", float64(sl.Speed)/8.0)
	result := GostUpdateLimiters(tunnel.InNodeID, sl.ID, speed)
	if IsNotFound(result) {
		GostAddLimiters(tunnel.InNodeID, sl.ID, speed)
	} else if !IsOK(result) {
		Rerr(c, result.Msg)
		return
	}

	DB.Save(&sl)
	RokMsg(c)
}

func HandleSpeedLimitDelete(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	id, _ := getParamInt64(params, "id")

	var sl SpeedLimit
	if err := DB.First(&sl, id).Error; err != nil {
		Rerr(c, "限速规则不存在")
		return
	}

	// Check usage
	var count int64
	DB.Model(&UserTunnel{}).Where("speed_id = ?", id).Count(&count)
	if count > 0 {
		Rerr(c, "该限速规则还有用户在使用 请先取消分配")
		return
	}

	// Delete limiter in GOST
	var tunnel Tunnel
	if DB.First(&tunnel, sl.TunnelID).Error == nil {
		GostDeleteLimiters(tunnel.InNodeID, id)
	}

	DB.Delete(&SpeedLimit{}, id)
	RokMsg(c)
}
