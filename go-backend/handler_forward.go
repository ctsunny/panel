package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// -- Forward Handlers --

func HandleForwardCreate(c *gin.Context) {
	var dto CreateForwardDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	userID := GetCurrentUserID(c)
	roleID := GetCurrentRoleID(c)

	// Validate tunnel
	var tunnel Tunnel
	if err := DB.First(&tunnel, dto.TunnelID).Error; err != nil {
		Rerr(c, "隧道不存在")
		return
	}
	if tunnel.Status != 1 {
		Rerr(c, "隧道已禁用，无法创建转发")
		return
	}

	// Get user info
	var user User
	if err := DB.First(&user, userID).Error; err != nil {
		Rerr(c, "用户不存在")
		return
	}

	// Check user tunnel permission (non-admin)
	var userTunnelID int
	var limiterID *int64
	if roleID != adminRoleID {
		var ut UserTunnel
		if err := DB.Where("user_id = ? AND tunnel_id = ?", userID, dto.TunnelID).First(&ut).Error; err != nil {
			Rerr(c, "你没有该隧道权限")
			return
		}
		if ut.Status != 1 {
			Rerr(c, "隧道被禁用")
			return
		}
		// Check expiry
		if ut.ExpTime > 0 && ut.ExpTime <= nowMs() {
			Rerr(c, "用户的该隧道权限已到期")
			return
		}
		// Check forward count limit
		var fwdCount int64
		DB.Model(&Forward{}).Where("user_id = ? AND tunnel_id = ?", userID, dto.TunnelID).Count(&fwdCount)
		if ut.Num > 0 && int(fwdCount) >= ut.Num {
			Rerr(c, "转发数量已达上限")
			return
		}

		userTunnelID = ut.ID
		limiterID = ut.SpeedID

		// Check user flow and expiry
		if err := checkUserFlowLimits(&user); err != nil {
			Rerr(c, err.Error())
			return
		}
	}

	// Allocate port
	inPort, err := allocatePort(&tunnel, dto.InPort)
	if err != nil {
		Rerr(c, err.Error())
		return
	}

	now := nowMs()
	strategy := dto.Strategy
	if strategy == "" {
		strategy = "fifo"
	}

	forward := &Forward{
		UserID:        userID,
		UserName:      user.User,
		Name:          dto.Name,
		TunnelID:      dto.TunnelID,
		InPort:        inPort,
		OutPort:       dto.OutPort,
		RemoteAddr:    dto.RemoteAddr,
		Strategy:      strategy,
		InterfaceName: dto.InterfaceName,
		CreatedTime:   now,
		UpdatedTime:   now,
		Status:        1,
	}
	if err := DB.Create(forward).Error; err != nil {
		Rerr(c, "端口转发创建失败")
		return
	}

	// Get nodes
	var inNode Node
	if err := DB.First(&inNode, tunnel.InNodeID).Error; err != nil {
		DB.Delete(&Forward{}, forward.ID)
		Rerr(c, "入口节点不存在")
		return
	}

	sn := ServiceName(forward.ID, userID, userTunnelID)

	// Add limiter if needed
	if limiterID != nil {
		var sl SpeedLimit
		if DB.First(&sl, *limiterID).Error == nil {
			speed := fmt.Sprintf("%.1f", float64(sl.Speed)/8.0)
			GostAddLimiters(tunnel.InNodeID, *limiterID, speed)
		}
	}

	// Create GOST services
	if tunnel.Type == 1 {
		// Port forward
		result := GostAddService(tunnel.InNodeID, sn, inPort, limiterID, dto.RemoteAddr, tunnel.Flow, &tunnel, strategy, dto.InterfaceName)
		if !IsOK(result) {
			DB.Delete(&Forward{}, forward.ID)
			Rerr(c, result.Msg)
			return
		}
	} else {
		// Tunnel forward
		proto := "tls"
		if tunnel.Protocol != nil && *tunnel.Protocol != "" {
			proto = *tunnel.Protocol
		}
		outPort := inPort
		if dto.OutPort != nil {
			outPort = *dto.OutPort
		}

		_ = inNode
		// Add chains on in-node
		remoteAddrForChain := fmt.Sprintf("%s:%d", tunnel.OutIP, outPort)
		result := GostAddChains(tunnel.InNodeID, sn, remoteAddrForChain, proto, dto.InterfaceName)
		if !IsOK(result) {
			DB.Delete(&Forward{}, forward.ID)
			Rerr(c, result.Msg)
			return
		}
		// Add service on in-node
		result = GostAddService(tunnel.InNodeID, sn, inPort, limiterID, dto.RemoteAddr, tunnel.Flow, &tunnel, strategy, dto.InterfaceName)
		if !IsOK(result) {
			GostDeleteChains(tunnel.InNodeID, sn)
			DB.Delete(&Forward{}, forward.ID)
			Rerr(c, result.Msg)
			return
		}
		// Add remote service on out-node
		result = GostAddRemoteService(tunnel.OutNodeID, sn, outPort, dto.RemoteAddr, proto, strategy, dto.InterfaceName)
		if !IsOK(result) {
			GostDeleteService(tunnel.InNodeID, sn)
			GostDeleteChains(tunnel.InNodeID, sn)
			DB.Delete(&Forward{}, forward.ID)
			Rerr(c, result.Msg)
			return
		}
	}

	RokMsg(c)
}

func HandleForwardList(c *gin.Context) {
	userID := GetCurrentUserID(c)
	roleID := GetCurrentRoleID(c)

	var results []ForwardWithTunnelDTO
	query := DB.Table("forwards").
		Select(`forwards.id, forwards.user_id, forwards.user_name, forwards.name, forwards.tunnel_id,
		        forwards.in_port, forwards.out_port, forwards.remote_addr, forwards.strategy,
		        forwards.interface_name, forwards.in_flow, forwards.out_flow, forwards.status,
		        forwards.created_time, forwards.updated_time, forwards.inx,
		        tunnels.name as tunnel_name, tunnels.in_ip, tunnels.out_ip, tunnels.type, tunnels.protocol`).
		Joins("LEFT JOIN tunnels ON forwards.tunnel_id = tunnels.id").
		Order("forwards.created_time DESC")

	if roleID != adminRoleID {
		query = query.Where("forwards.user_id = ?", userID)
	}

	query.Scan(&results)
	Rok(c, results)
}

func HandleForwardUpdate(c *gin.Context) {
	var dto UpdateForwardDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	userID := GetCurrentUserID(c)
	roleID := GetCurrentRoleID(c)

	// Check user status
	if roleID != adminRoleID {
		var user User
		if err := DB.First(&user, userID).Error; err != nil {
			Rerr(c, "用户不存在")
			return
		}
		if user.Status == 0 {
			Rerr(c, "用户已到期或被禁用")
			return
		}
	}

	// Get existing forward
	var forward Forward
	q := DB.First(&forward, dto.ID)
	if roleID != adminRoleID {
		q = DB.Where("id = ? AND user_id = ?", dto.ID, userID).First(&forward)
	}
	if q.Error != nil {
		Rerr(c, "转发不存在")
		return
	}

	// Get tunnel
	tunnelID := dto.TunnelID
	if tunnelID == 0 {
		tunnelID = forward.TunnelID
	}
	var tunnel Tunnel
	if err := DB.First(&tunnel, tunnelID).Error; err != nil {
		Rerr(c, "隧道不存在")
		return
	}
	if tunnel.Status != 1 {
		Rerr(c, "隧道已禁用，无法更新转发")
		return
	}

	// Get UserTunnel for the owner of the forward
	ownerID := forward.UserID
	var ut *UserTunnel
	if roleID != adminRoleID {
		ownerID = userID
	}
	var utObj UserTunnel
	if DB.Where("user_id = ? AND tunnel_id = ?", ownerID, tunnelID).First(&utObj).Error == nil {
		ut = &utObj
	}

	// Apply updates to forward entity
	if dto.Name != "" {
		forward.Name = dto.Name
	}
	if dto.TunnelID != 0 {
		forward.TunnelID = dto.TunnelID
	}
	if dto.RemoteAddr != "" {
		forward.RemoteAddr = dto.RemoteAddr
	}
	if dto.Strategy != "" {
		forward.Strategy = dto.Strategy
	}
	if dto.InPort != nil {
		forward.InPort = *dto.InPort
	}
	if dto.OutPort != nil {
		forward.OutPort = dto.OutPort
	}
	if dto.InterfaceName != nil {
		forward.InterfaceName = dto.InterfaceName
	}
	forward.UpdatedTime = nowMs()

	var limiterID *int64
	if ut != nil {
		limiterID = ut.SpeedID
	}

	// Update GOST services (limiterID is used inside updateForwardGost via getUserTunnelID)
	if err := updateForwardGostWithLimiter(&forward, &tunnel, dto, limiterID); err != nil {
		// Still save the forward even if GOST fails
	}

	forward.Status = 1
	DB.Save(&forward)
	RokMsg(c)
}

func updateForwardGost(forward *Forward, tunnel *Tunnel, dto UpdateForwardDTO) error {
	var ut *UserTunnel
	var utObj UserTunnel
	if DB.Where("user_id = ? AND tunnel_id = ?", forward.UserID, tunnel.ID).First(&utObj).Error == nil {
		ut = &utObj
	}
	var limiterID *int64
	if ut != nil {
		limiterID = ut.SpeedID
	}
	return updateForwardGostWithLimiter(forward, tunnel, dto, limiterID)
}

func updateForwardGostWithLimiter(forward *Forward, tunnel *Tunnel, dto UpdateForwardDTO, limiterID *int64) error {
	ownerID := forward.UserID
	utID := getUserTunnelID(ownerID, tunnel.ID)
	sn := ServiceName(forward.ID, forward.UserID, utID)
	_ = dto

	if tunnel.Type == 1 {
		GostUpdateService(tunnel.InNodeID, sn, forward.InPort, limiterID, forward.RemoteAddr, tunnel.Flow, tunnel, forward.Strategy, forward.InterfaceName)
	} else {
		proto := "tls"
		if tunnel.Protocol != nil && *tunnel.Protocol != "" {
			proto = *tunnel.Protocol
		}
		outPort := forward.InPort
		if forward.OutPort != nil {
			outPort = *forward.OutPort
		}
		remoteForChain := fmt.Sprintf("%s:%d", tunnel.OutIP, outPort)
		GostUpdateChains(tunnel.InNodeID, sn, remoteForChain, proto, forward.InterfaceName)
		GostUpdateService(tunnel.InNodeID, sn, forward.InPort, limiterID, forward.RemoteAddr, tunnel.Flow, tunnel, forward.Strategy, forward.InterfaceName)
		GostUpdateRemoteService(tunnel.OutNodeID, sn, outPort, forward.RemoteAddr, proto, forward.Strategy, forward.InterfaceName)
	}
	return nil
}

func HandleForwardDelete(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	id, _ := getParamInt64(params, "id")
	userID := GetCurrentUserID(c)
	roleID := GetCurrentRoleID(c)

	var forward Forward
	var err error
	if roleID == adminRoleID {
		err = DB.First(&forward, id).Error
	} else {
		err = DB.Where("id = ? AND user_id = ?", id, userID).First(&forward).Error
	}
	if err != nil {
		Rerr(c, "端口转发不存在")
		return
	}

	deleteForwardGost(&forward)
	DB.Delete(&Forward{}, id)
	RokMsg(c)
}

// deleteForwardGost deletes GOST services for a forward
func deleteForwardGost(forward *Forward) {
	var tunnel Tunnel
	if err := DB.First(&tunnel, forward.TunnelID).Error; err != nil {
		return
	}

	utID := getUserTunnelID(forward.UserID, forward.TunnelID)
	sn := ServiceName(forward.ID, forward.UserID, utID)

	GostDeleteService(tunnel.InNodeID, sn)
	if tunnel.Type == 2 {
		GostDeleteRemoteService(tunnel.OutNodeID, sn)
		GostDeleteChains(tunnel.InNodeID, sn)
	}
}

func HandleForwardForceDelete(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	id, _ := getParamInt64(params, "id")
	userID := GetCurrentUserID(c)
	roleID := GetCurrentRoleID(c)

	var forward Forward
	var err error
	if roleID == adminRoleID {
		err = DB.First(&forward, id).Error
	} else {
		err = DB.Where("id = ? AND user_id = ?", id, userID).First(&forward).Error
	}
	if err != nil {
		Rerr(c, "端口转发不存在")
		return
	}

	DB.Delete(&Forward{}, id)
	RokMsg(c)
}

func HandleForwardPause(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	id, _ := getParamInt64(params, "id")
	changeForwardStatus(c, id, 0)
}

func HandleForwardResume(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	id, _ := getParamInt64(params, "id")
	changeForwardStatus(c, id, 1)
}

func changeForwardStatus(c *gin.Context, id int64, targetStatus int) {
	userID := GetCurrentUserID(c)
	roleID := GetCurrentRoleID(c)

	if roleID != adminRoleID {
		var user User
		if err := DB.First(&user, userID).Error; err != nil {
			Rerr(c, "用户不存在")
			return
		}
		if user.Status == 0 {
			Rerr(c, "用户已到期或被禁用")
			return
		}
	}

	var forward Forward
	var err error
	if roleID == adminRoleID {
		err = DB.First(&forward, id).Error
	} else {
		err = DB.Where("id = ? AND user_id = ?", id, userID).First(&forward).Error
	}
	if err != nil {
		Rerr(c, "转发不存在")
		return
	}

	var tunnel Tunnel
	if err := DB.First(&tunnel, forward.TunnelID).Error; err != nil {
		Rerr(c, "隧道不存在")
		return
	}

	// Resume requires additional checks
	if targetStatus == 1 {
		if tunnel.Status != 1 {
			Rerr(c, "隧道已禁用，无法恢复服务")
			return
		}
		if roleID != adminRoleID {
			if err := checkUserFlowLimits2(userID); err != nil {
				Rerr(c, err.Error())
				return
			}
		}
	}

	utID := getUserTunnelID(forward.UserID, tunnel.ID)
	sn := ServiceName(forward.ID, forward.UserID, utID)

	if targetStatus == 0 {
		GostPauseService(tunnel.InNodeID, sn)
		if tunnel.Type == 2 {
			GostPauseRemoteService(tunnel.OutNodeID, sn)
		}
	} else {
		GostResumeService(tunnel.InNodeID, sn)
		if tunnel.Type == 2 {
			GostResumeRemoteService(tunnel.OutNodeID, sn)
		}
	}

	DB.Model(&Forward{}).Where("id = ?", id).Update("status", targetStatus)
	RokMsg(c)
}

func HandleForwardDiagnose(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	fwdID, _ := getParamInt64(params, "forwardId")

	var forward Forward
	if err := DB.First(&forward, fwdID).Error; err != nil {
		Rerr(c, "转发不存在")
		return
	}
	var tunnel Tunnel
	if err := DB.First(&tunnel, forward.TunnelID).Error; err != nil {
		Rerr(c, "隧道不存在")
		return
	}

	utID := getUserTunnelID(forward.UserID, tunnel.ID)
	sn := ServiceName(forward.ID, forward.UserID, utID)

	result := Hub.SendMsg(tunnel.InNodeID, map[string]interface{}{
		"serviceName": sn,
	}, "DiagnoseService")

	Rok(c, gin.H{"result": result.Msg})
}

func HandleForwardUpdateOrder(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}

	forwards, ok := params["forwards"].([]interface{})
	if !ok {
		Rerr(c, "forwards参数错误")
		return
	}

	for _, item := range forwards {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id, err := getParamInt64(m, "id")
		if err != nil {
			continue
		}
		inx, _ := getParamInt64(m, "inx")
		DB.Model(&Forward{}).Where("id = ?", id).Update("inx", inx)
	}
	RokMsg(c)
}

// allocatePort finds an available port in the tunnel's range
func allocatePort(tunnel *Tunnel, requestedPort *int) (int, error) {
	var inNode Node
	if err := DB.First(&inNode, tunnel.InNodeID).Error; err != nil {
		return 0, fmt.Errorf("入口节点不存在")
	}

	if requestedPort != nil && *requestedPort > 0 {
		// Validate port is in range
		if *requestedPort < inNode.PortSta || *requestedPort > inNode.PortEnd {
			return 0, fmt.Errorf("端口不在节点允许范围内 (%d-%d)", inNode.PortSta, inNode.PortEnd)
		}
		// Check not already in use
		var count int64
		DB.Model(&Forward{}).
			Joins("JOIN tunnels ON forwards.tunnel_id = tunnels.id").
			Where("tunnels.in_node_id = ? AND forwards.in_port = ?", inNode.ID, *requestedPort).
			Count(&count)
		if count > 0 {
			return 0, fmt.Errorf("端口 %d 已被占用", *requestedPort)
		}
		return *requestedPort, nil
	}

	// Auto-assign: find first available port
	var usedPorts []int
	DB.Model(&Forward{}).
		Select("forwards.in_port").
		Joins("JOIN tunnels ON forwards.tunnel_id = tunnels.id").
		Where("tunnels.in_node_id = ?", inNode.ID).
		Pluck("forwards.in_port", &usedPorts)

	usedSet := make(map[int]bool)
	for _, p := range usedPorts {
		usedSet[p] = true
	}

	for p := inNode.PortSta; p <= inNode.PortEnd; p++ {
		if !usedSet[p] {
			return p, nil
		}
	}
	return 0, fmt.Errorf("节点端口已耗尽")
}

func checkUserFlowLimits(user *User) error {
	if user.Flow > 0 {
		used := user.InFlow + user.OutFlow
		limit := user.Flow * 1024 * 1024 * 1024
		if used >= limit {
			return fmt.Errorf("用户流量已超限")
		}
	}
	if user.ExpTime > 0 && user.ExpTime <= nowMs() {
		return fmt.Errorf("用户账号已过期")
	}
	if user.Status != 1 {
		return fmt.Errorf("用户账号已禁用")
	}
	return nil
}

func checkUserFlowLimits2(userID int64) error {
	var user User
	if err := DB.First(&user, userID).Error; err != nil {
		return fmt.Errorf("用户不存在")
	}
	return checkUserFlowLimits(&user)
}
