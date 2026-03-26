package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// -- Tunnel Handlers --

func HandleTunnelCreate(c *gin.Context) {
	var dto CreateTunnelDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	// Check name uniqueness
	var count int64
	DB.Model(&Tunnel{}).Where("name = ?", dto.Name).Count(&count)
	if count > 0 {
		Rerr(c, "隧道名称已存在")
		return
	}

	// Validate tunnel forward requirements
	if dto.Type == 2 && dto.OutNodeID == nil {
		Rerr(c, "出口节点不能为空")
		return
	}

	// Validate in-node
	var inNode Node
	if err := DB.First(&inNode, dto.InNodeID).Error; err != nil {
		Rerr(c, "入口节点不存在")
		return
	}
	if inNode.Status != 1 {
		Rerr(c, "入口节点当前离线，请确保节点正常运行")
		return
	}

	now := nowMs()
	tunnel := &Tunnel{
		Name:         dto.Name,
		TrafficRatio: dto.TrafficRatio,
		InNodeID:     dto.InNodeID,
		InIP:         ptrToStr(inNode.IP),
		Flow:         dto.Flow,
		CreatedTime:  now,
		UpdatedTime:  now,
		Status:       1,
	}
	if tunnel.TrafficRatio == 0 {
		tunnel.TrafficRatio = 1.0
	}

	tcpAddr := dto.TCPListenAddr
	if tcpAddr == "" {
		tcpAddr = "0.0.0.0"
	}
	udpAddr := dto.UDPListenAddr
	if udpAddr == "" {
		udpAddr = "0.0.0.0"
	}
	tunnel.TCPListenAddr = tcpAddr
	tunnel.UDPListenAddr = udpAddr
	tunnel.InterfaceName = dto.InterfaceName

	if dto.Type == 1 {
		// Port forward: out = in node
		tunnel.Type = 1
		tunnel.OutNodeID = dto.InNodeID
		tunnel.OutIP = inNode.ServerIP
		tunnel.Protocol = nil
	} else {
		// Tunnel forward
		tunnel.Type = 2
		var outNode Node
		if err := DB.First(&outNode, *dto.OutNodeID).Error; err != nil {
			Rerr(c, "出口节点不存在")
			return
		}
		if outNode.Status != 1 {
			Rerr(c, "出口节点当前离线，请确保节点正常运行")
			return
		}
		if outNode.ID == inNode.ID {
			Rerr(c, "隧道转发模式下，入口和出口不能是同一个节点")
			return
		}
		tunnel.OutNodeID = outNode.ID
		tunnel.OutIP = outNode.ServerIP
		proto := dto.Protocol
		if proto == "" {
			proto = "tls"
		}
		tunnel.Protocol = &proto
	}

	if err := DB.Create(tunnel).Error; err != nil {
		Rerr(c, "隧道创建失败")
		return
	}
	RokMsg(c)
}

func HandleTunnelList(c *gin.Context) {
	var tunnels []Tunnel
	DB.Find(&tunnels)
	Rok(c, tunnels)
}

func HandleTunnelUpdate(c *gin.Context) {
	var dto UpdateTunnelDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	var tunnel Tunnel
	if err := DB.First(&tunnel, dto.ID).Error; err != nil {
		Rerr(c, "隧道不存在")
		return
	}

	// Check name uniqueness (exclude self)
	if dto.Name != "" && dto.Name != tunnel.Name {
		var cnt int64
		DB.Model(&Tunnel{}).Where("name = ? AND id != ?", dto.Name, dto.ID).Count(&cnt)
		if cnt > 0 {
			Rerr(c, "隧道名称已存在")
			return
		}
	}

	// Detect if critical fields changed (need forward re-sync)
	needSync := (dto.TCPListenAddr != "" && dto.TCPListenAddr != tunnel.TCPListenAddr) ||
		(dto.UDPListenAddr != "" && dto.UDPListenAddr != tunnel.UDPListenAddr) ||
		(dto.Protocol != "" && (tunnel.Protocol == nil || dto.Protocol != *tunnel.Protocol)) ||
		(dto.InterfaceName != nil && (tunnel.InterfaceName == nil || *dto.InterfaceName != *tunnel.InterfaceName))

	// Apply updates
	if dto.Name != "" {
		tunnel.Name = dto.Name
	}
	if dto.Flow != 0 {
		tunnel.Flow = dto.Flow
	}
	if dto.TCPListenAddr != "" {
		tunnel.TCPListenAddr = dto.TCPListenAddr
	}
	if dto.UDPListenAddr != "" {
		tunnel.UDPListenAddr = dto.UDPListenAddr
	}
	if dto.TrafficRatio != 0 {
		tunnel.TrafficRatio = dto.TrafficRatio
	}
	if dto.Protocol != "" {
		tunnel.Protocol = &dto.Protocol
	}
	if dto.InterfaceName != nil {
		tunnel.InterfaceName = dto.InterfaceName
	}
	if dto.Status != 0 {
		tunnel.Status = dto.Status
	}
	DB.Save(&tunnel)

	// Re-sync forwards if critical fields changed
	if needSync {
		var forwards []Forward
		DB.Where("tunnel_id = ?", dto.ID).Find(&forwards)
		for _, f := range forwards {
			// Build update DTO and re-apply
			fDto := UpdateForwardDTO{
				ID:            f.ID,
				UserID:        f.UserID,
				TunnelID:      f.TunnelID,
				Name:          f.Name,
				RemoteAddr:    f.RemoteAddr,
				Strategy:      f.Strategy,
				InPort:        &f.InPort,
				InterfaceName: f.InterfaceName,
			}
			if f.OutPort != nil {
				fDto.OutPort = f.OutPort
			}
			updateForwardGost(&f, &tunnel, fDto)
		}
	}

	RokMsg(c)
}

func HandleTunnelDelete(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	id, _ := getParamInt64(params, "id")

	var tunnel Tunnel
	if err := DB.First(&tunnel, id).Error; err != nil {
		Rerr(c, "隧道不存在")
		return
	}

	// Check usage
	var fwdCount, utCount int64
	DB.Model(&Forward{}).Where("tunnel_id = ?", id).Count(&fwdCount)
	DB.Model(&UserTunnel{}).Where("tunnel_id = ?", id).Count(&utCount)
	if fwdCount > 0 {
		Rerr(c, fmt.Sprintf("该隧道还有 %d 个转发在使用，请先删除相关转发", fwdCount))
		return
	}
	if utCount > 0 {
		Rerr(c, fmt.Sprintf("该隧道还有 %d 个用户权限关联，请先取消用户权限分配", utCount))
		return
	}

	DB.Delete(&Tunnel{}, id)
	RokMsg(c)
}

// UserTunnel accessible tunnels
func HandleTunnelUserTunnel(c *gin.Context) {
	userID := GetCurrentUserID(c)
	roleID := GetCurrentRoleID(c)

	var tunnels []Tunnel
	if roleID == adminRoleID {
		DB.Where("status = 1").Find(&tunnels)
	} else {
		// Only return tunnels the user has permission for
		var userTunnels []UserTunnel
		DB.Where("user_id = ? AND status = 1", userID).Find(&userTunnels)
		tunnelIDs := make([]int64, 0, len(userTunnels))
		for _, ut := range userTunnels {
			tunnelIDs = append(tunnelIDs, ut.TunnelID)
		}
		if len(tunnelIDs) > 0 {
			DB.Where("id IN ? AND status = 1", tunnelIDs).Find(&tunnels)
		}
	}

	dtos := make([]TunnelListDTO, 0, len(tunnels))
	for _, t := range tunnels {
		dtos = append(dtos, TunnelListDTO{
			ID:            t.ID,
			Name:          t.Name,
			Type:          t.Type,
			Protocol:      t.Protocol,
			Flow:          t.Flow,
			TCPListenAddr: t.TCPListenAddr,
			UDPListenAddr: t.UDPListenAddr,
			InNodeID:      t.InNodeID,
			OutNodeID:     t.OutNodeID,
			Status:        t.Status,
		})
	}
	Rok(c, dtos)
}

func HandleTunnelDiagnose(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	tunnelID, _ := getParamInt64(params, "tunnelId")

	var tunnel Tunnel
	if err := DB.First(&tunnel, tunnelID).Error; err != nil {
		Rerr(c, "隧道不存在")
		return
	}

	result := Hub.SendMsg(tunnel.InNodeID, map[string]interface{}{
		"tunnelId": tunnelID,
	}, "DiagnoseTunnel")

	Rok(c, gin.H{"result": result.Msg})
}

func ptrToStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
