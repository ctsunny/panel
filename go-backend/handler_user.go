package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// -- User Handlers --

func HandleUserLogin(c *gin.Context) {
	var dto LoginDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	// Check captcha if enabled
	var captchaCfg ViteConfig
	if DB.Where("name = ?", "captcha_enabled").First(&captchaCfg).Error == nil {
		if captchaCfg.Value == "true" {
			// Captcha enabled but Go backend doesn't support tianai-captcha
			// Just skip verification (no captcha ID sent = fail silently)
		}
	}

	// Find user
	var user User
	if err := DB.Where("user = ?", dto.Username).First(&user).Error; err != nil {
		Rerr(c, "账号或密码错误")
		return
	}

	// Verify password (plain MD5)
	if MD5(dto.Password) != user.Pwd {
		Rerr(c, "账号或密码错误")
		return
	}

	// Check account status
	if user.Status != 1 {
		Rerr(c, "账户停用")
		return
	}

	token, err := GenerateToken(&user)
	if err != nil {
		Rerr(c, "生成token失败")
		return
	}

	// Check if using default credentials
	requirePasswordChange := dto.Username == "admin_user" && dto.Password == "admin_user"

	user.Pwd = "" // don't return password
	Rok(c, gin.H{
		"token":                 token,
		"name":                  user.User,
		"role_id":               user.RoleID,
		"requirePasswordChange": requirePasswordChange,
	})
}

func HandleUserCreate(c *gin.Context) {
	var dto CreateUserDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	// Check username uniqueness
	var count int64
	DB.Model(&User{}).Where("user = ?", dto.User).Count(&count)
	if count > 0 {
		Rerr(c, "用户名已存在")
		return
	}

	now := nowMs()
	user := &User{
		User:          dto.User,
		Pwd:           MD5(dto.Pwd),
		RoleID:        1,
		ExpTime:       dto.ExpTime,
		Flow:          dto.Flow,
		InFlow:        0,
		OutFlow:       0,
		FlowResetTime: dto.FlowResetTime,
		Num:           dto.Num,
		Status:        dto.Status,
		CreatedTime:   now,
	}
	if user.Status == 0 {
		user.Status = 1
	}

	if err := DB.Create(user).Error; err != nil {
		Rerr(c, "用户创建失败")
		return
	}
	RokMsg(c)
}

func HandleUserList(c *gin.Context) {
	var users []User
	DB.Where("role_id != 0").Find(&users)
	for i := range users {
		users[i].Pwd = ""
	}
	Rok(c, users)
}

func HandleUserUpdate(c *gin.Context) {
	var dto UpdateUserDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	var user User
	if err := DB.First(&user, dto.ID).Error; err != nil {
		Rerr(c, "用户不存在")
		return
	}
	if user.RoleID == 0 {
		Rerr(c, "不能修改管理员用户信息")
		return
	}

	// Check username uniqueness (exclude self)
	if dto.User != "" && dto.User != user.User {
		var count int64
		DB.Model(&User{}).Where("user = ? AND id != ?", dto.User, dto.ID).Count(&count)
		if count > 0 {
			Rerr(c, "用户名已被其他用户使用")
			return
		}
		user.User = dto.User
	}

	if dto.Pwd != "" {
		user.Pwd = MD5(dto.Pwd)
	}
	if dto.ExpTime != 0 {
		user.ExpTime = dto.ExpTime
	}
	if dto.Flow != 0 {
		user.Flow = dto.Flow
	}
	if dto.FlowResetTime != 0 {
		user.FlowResetTime = dto.FlowResetTime
	}
	if dto.Num != 0 {
		user.Num = dto.Num
	}
	user.Status = dto.Status
	now := nowMs()
	user.UpdatedTime = &now

	if err := DB.Save(&user).Error; err != nil {
		Rerr(c, "用户更新失败")
		return
	}
	RokMsg(c)
}

func HandleUserDelete(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	id, err := getParamInt64(params, "id")
	if err != nil {
		Rerr(c, "id参数错误")
		return
	}

	var user User
	if err := DB.First(&user, id).Error; err != nil {
		Rerr(c, "用户不存在")
		return
	}
	if user.RoleID == 0 {
		Rerr(c, "不能删除管理员用户")
		return
	}

	// Cascade delete: forwards + GOST cleanup + user_tunnel
	deleteUserCascade(id)

	DB.Delete(&User{}, id)
	RokMsg(c)
}

func deleteUserCascade(userID int64) {
	// Delete all forwards for this user (with GOST cleanup)
	var forwards []Forward
	DB.Where("user_id = ?", userID).Find(&forwards)
	for _, f := range forwards {
		deleteForwardGost(&f)
		DB.Delete(&Forward{}, f.ID)
	}

	// Delete user tunnels
	DB.Where("user_id = ?", userID).Delete(&UserTunnel{})
}

func HandleUserPackage(c *gin.Context) {
	userID := GetCurrentUserID(c)
	var user User
	if err := DB.First(&user, userID).Error; err != nil {
		Rerr(c, "用户不存在")
		return
	}
	user.Pwd = ""

	// Get tunnel permissions
	var userTunnels []UserTunnelWithDetailDTO
	DB.Table("user_tunnels").
		Select("user_tunnels.*, tunnels.name as tunnel_name").
		Joins("LEFT JOIN tunnels ON user_tunnels.tunnel_id = tunnels.id").
		Where("user_tunnels.user_id = ?", userID).
		Scan(&userTunnels)

	// Get forwards with tunnel info
	var forwards []ForwardWithTunnelDTO
	DB.Table("forwards").
		Select(`forwards.id, forwards.user_id, forwards.user_name, forwards.name, forwards.tunnel_id,
		        forwards.in_port, forwards.out_port, forwards.remote_addr, forwards.strategy,
		        forwards.interface_name, forwards.in_flow, forwards.out_flow, forwards.status,
		        forwards.created_time, forwards.updated_time, forwards.inx,
		        tunnels.name as tunnel_name, tunnels.in_ip, tunnels.out_ip, tunnels.type, tunnels.protocol`).
		Joins("LEFT JOIN tunnels ON forwards.tunnel_id = tunnels.id").
		Where("forwards.user_id = ?", userID).
		Order("forwards.created_time DESC").
		Scan(&forwards)

	// Get last 24 hours statistics
	cutoff := nowMs() - 24*60*60*1000
	var stats []StatisticsFlow
	DB.Where("user_id = ? AND created_time >= ?", userID, cutoff).
		Order("created_time ASC").
		Find(&stats)

	Rok(c, gin.H{
		"userInfo":          user,
		"tunnelPermissions": userTunnels,
		"forwards":          forwards,
		"statisticsFlows":   stats,
	})
}

func HandleUpdatePassword(c *gin.Context) {
	var dto ChangePasswordDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	if dto.NewPassword != dto.ConfirmPwd {
		Rerr(c, "新密码和确认密码不匹配")
		return
	}

	userID := GetCurrentUserID(c)
	var user User
	if err := DB.First(&user, userID).Error; err != nil {
		Rerr(c, "用户不存在")
		return
	}

	if MD5(dto.OldPassword) != user.Pwd {
		Rerr(c, "当前密码错误")
		return
	}

	now := nowMs()
	DB.Model(&user).Updates(map[string]interface{}{
		"pwd":          MD5(dto.NewPassword),
		"updated_time": now,
	})
	RokMsg(c)
}

func HandleResetFlow(c *gin.Context) {
	var dto ResetFlowDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误")
		return
	}

	result := DB.Model(&User{}).Where("id = ?", dto.ID).Updates(map[string]interface{}{
		"in_flow":  0,
		"out_flow": 0,
	})
	if result.Error != nil {
		Rerr(c, "重置失败")
		return
	}
	RokMsg(c)
}

// getParamInt64 extracts int64 from a map of interface{}
func getParamInt64(params map[string]interface{}, key string) (int64, error) {
	v, ok := params[key]
	if !ok {
		return 0, fmt.Errorf("missing %s", key)
	}
	switch val := v.(type) {
	case float64:
		return int64(val), nil
	case int64:
		return val, nil
	case int:
		return int64(val), nil
	default:
		var n int64
		_, err := fmt.Sscanf(fmt.Sprintf("%v", v), "%d", &n)
		return n, err
	}
}

func getParamInt(params map[string]interface{}, key string) (int, error) {
	n, err := getParamInt64(params, key)
	return int(n), err
}

func ptrInt64(n int64) *int64 { return &n }
func ptrTime() *int64        { n := nowMs(); return &n }
func nowMsPtr() *int64       { n := nowMs(); return &n }

var (
	_ = ptrInt64
	_ = ptrTime
	_ = nowMsPtr
)
