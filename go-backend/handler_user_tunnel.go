package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// -- UserTunnel Handlers --

func HandleUserTunnelAssign(c *gin.Context) {
	var dto AssignUserTunnelDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	// Check if permission already exists
	var count int64
	DB.Model(&UserTunnel{}).Where("user_id = ? AND tunnel_id = ?", dto.UserID, dto.TunnelID).Count(&count)
	if count > 0 {
		Rerr(c, "该用户已拥有此隧道权限")
		return
	}

	ut := &UserTunnel{
		UserID:        dto.UserID,
		TunnelID:      dto.TunnelID,
		SpeedID:       dto.SpeedID,
		Num:           dto.Num,
		Flow:          dto.Flow,
		InFlow:        0,
		OutFlow:       0,
		FlowResetTime: dto.FlowResetTime,
		ExpTime:       dto.ExpTime,
		Status:        1,
	}
	if dto.Status != 0 {
		ut.Status = dto.Status
	}

	if err := DB.Create(ut).Error; err != nil {
		Rerr(c, "用户隧道权限分配失败")
		return
	}
	RokMsg(c)
}

func HandleUserTunnelList(c *gin.Context) {
	var params struct {
		UserID int64 `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}

	// Join UserTunnel with Tunnel to get tunnel name
	type Result struct {
		UserTunnelWithDetailDTO
	}

	var results []UserTunnelWithDetailDTO
	DB.Table("user_tunnels").
		Select("user_tunnels.*, tunnels.name as tunnel_name").
		Joins("LEFT JOIN tunnels ON user_tunnels.tunnel_id = tunnels.id").
		Where("user_tunnels.user_id = ?", params.UserID).
		Scan(&results)

	Rok(c, results)
}

func HandleUserTunnelRemove(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	id, _ := getParamInt(params, "id")

	var ut UserTunnel
	if err := DB.First(&ut, id).Error; err != nil {
		Rerr(c, "未找到对应的用户隧道权限记录")
		return
	}

	// Delete all forwards for this user in this tunnel
	var forwards []Forward
	DB.Where("user_id = ? AND tunnel_id = ?", ut.UserID, ut.TunnelID).Find(&forwards)
	for _, f := range forwards {
		deleteForwardGost(&f)
		DB.Delete(&Forward{}, f.ID)
	}

	DB.Delete(&UserTunnel{}, id)
	RokMsg(c)
}

func HandleUserTunnelUpdate(c *gin.Context) {
	var dto UpdateUserTunnelDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	var ut UserTunnel
	if err := DB.First(&ut, dto.ID).Error; err != nil {
		Rerr(c, "用户隧道权限不存在")
		return
	}

	oldSpeedID := ut.SpeedID

	ut.Flow = dto.Flow
	ut.Num = dto.Num
	if dto.FlowResetTime != nil {
		ut.FlowResetTime = *dto.FlowResetTime
	}
	if dto.ExpTime != nil {
		ut.ExpTime = *dto.ExpTime
	}
	if dto.Status != nil {
		ut.Status = *dto.Status
	}
	ut.SpeedID = dto.SpeedID

	if err := DB.Save(&ut).Error; err != nil {
		Rerr(c, "用户隧道权限更新失败")
		return
	}

	// If speed changed, update limiters on forwards
	speedChanged := !int64PtrEqual(oldSpeedID, dto.SpeedID)
	if speedChanged {
		updateUserTunnelForwardsSpeed(ut.UserID, ut.TunnelID, dto.SpeedID)
	}

	RokMsg(c)
}

func updateUserTunnelForwardsSpeed(userID, tunnelID int64, speedID *int64) {
	var tunnel Tunnel
	if err := DB.First(&tunnel, tunnelID).Error; err != nil {
		return
	}

	var forwards []Forward
	DB.Where("user_id = ? AND tunnel_id = ?", userID, tunnelID).Find(&forwards)

	for _, f := range forwards {
		if speedID != nil {
			var sl SpeedLimit
			if err := DB.First(&sl, *speedID).Error; err == nil {
				speed := fmt.Sprintf("%.1f", float64(sl.Speed)/8.0)
				sn := ServiceName(f.ID, f.UserID, getUserTunnelID(f.UserID, f.TunnelID))
				// Try update, then add if not found
				result := GostUpdateLimiters(tunnel.InNodeID, *speedID, speed)
				if IsNotFound(result) {
					GostAddLimiters(tunnel.InNodeID, *speedID, speed)
				}
				_ = sn // service name computed above
			}
		}
	}
}

func getUserTunnelID(userID, tunnelID int64) int {
	var ut UserTunnel
	if err := DB.Where("user_id = ? AND tunnel_id = ?", userID, tunnelID).First(&ut).Error; err != nil {
		return 0
	}
	return ut.ID
}

func int64PtrEqual(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
