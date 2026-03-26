package main

import (
	"crypto/rand"
	"fmt"

	"github.com/gin-gonic/gin"
)

// -- Node Handlers --

func HandleNodeCreate(c *gin.Context) {
	var dto CreateNodeDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	if err := validatePortRange(dto.PortSta, dto.PortEnd); err != nil {
		Rerr(c, err.Error())
		return
	}

	now := nowMs()
	node := &Node{
		Name:        dto.Name,
		Secret:      generateSecret(),
		IP:          &dto.IP,
		ServerIP:    dto.ServerIP,
		PortSta:     dto.PortSta,
		PortEnd:     dto.PortEnd,
		CreatedTime: now,
		Status:      0,
	}
	if err := DB.Create(node).Error; err != nil {
		Rerr(c, "节点创建失败")
		return
	}
	RokMsg(c)
}

func HandleNodeList(c *gin.Context) {
	var nodes []Node
	DB.Find(&nodes)
	// Hide secrets
	for i := range nodes {
		nodes[i].Secret = ""
	}
	Rok(c, nodes)
}

func HandleNodeUpdate(c *gin.Context) {
	var dto UpdateNodeDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		Rerr(c, "参数错误: "+err.Error())
		return
	}

	var node Node
	if err := DB.First(&node, dto.ID).Error; err != nil {
		Rerr(c, "节点不存在")
		return
	}

	// If node is online and protocol settings changed, notify via WS
	online := node.Status == 1
	if online && (dto.HTTP != nil || dto.TLS != nil || dto.Socks != nil) {
		httpV := node.HTTP
		tlsV := node.TLS
		socksV := node.Socks
		if dto.HTTP != nil {
			httpV = *dto.HTTP
		}
		if dto.TLS != nil {
			tlsV = *dto.TLS
		}
		if dto.Socks != nil {
			socksV = *dto.Socks
		}
		result := GostSetProtocol(node.ID, httpV, tlsV, socksV)
		if !IsOK(result) {
			Rerr(c, result.Msg)
			return
		}
	}

	// Update fields
	now := nowMs()
	updates := map[string]interface{}{"updated_time": now}
	if dto.Name != "" {
		updates["name"] = dto.Name
	}
	if dto.IP != "" {
		updates["ip"] = dto.IP
	}
	if dto.ServerIP != "" {
		updates["server_ip"] = dto.ServerIP
	}
	if dto.PortSta != 0 {
		updates["port_sta"] = dto.PortSta
	}
	if dto.PortEnd != 0 {
		updates["port_end"] = dto.PortEnd
	}
	if dto.HTTP != nil {
		updates["http"] = *dto.HTTP
	}
	if dto.TLS != nil {
		updates["tls"] = *dto.TLS
	}
	if dto.Socks != nil {
		updates["socks"] = *dto.Socks
	}
	DB.Model(&node).Updates(updates)

	// Update tunnel IPs that reference this node
	if dto.ServerIP != "" {
		DB.Model(&Tunnel{}).Where("out_node_id = ?", dto.ID).Update("out_ip", dto.ServerIP)
	}
	if dto.IP != "" {
		DB.Model(&Tunnel{}).Where("in_node_id = ?", dto.ID).Update("in_ip", dto.IP)
	}

	RokMsg(c)
}

func HandleNodeDelete(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	id, _ := getParamInt64(params, "id")

	var node Node
	if err := DB.First(&node, id).Error; err != nil {
		Rerr(c, "节点不存在")
		return
	}

	// Check if any tunnels use this node
	var inCount, outCount int64
	DB.Model(&Tunnel{}).Where("in_node_id = ?", id).Count(&inCount)
	DB.Model(&Tunnel{}).Where("out_node_id = ?", id).Count(&outCount)
	if inCount > 0 {
		Rerr(c, fmt.Sprintf("该节点还有 %d 个隧道作为入口节点在使用，请先删除相关隧道", inCount))
		return
	}
	if outCount > 0 {
		Rerr(c, fmt.Sprintf("该节点还有 %d 个隧道作为出口节点在使用，请先删除相关隧道", outCount))
		return
	}

	DB.Delete(&Node{}, id)
	RokMsg(c)
}

func HandleNodeInstall(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		Rerr(c, "参数错误")
		return
	}
	id, _ := getParamInt64(params, "id")

	var node Node
	if err := DB.First(&node, id).Error; err != nil {
		Rerr(c, "节点不存在")
		return
	}

	// Get server_addr from config
	var cfg ViteConfig
	if err := DB.Where("name = ?", "server_addr").First(&cfg).Error; err != nil {
		Rerr(c, "请先前往网站配置中设置ip")
		return
	}

	serverAddr := processServerAddress(cfg.Value)
	cmd := fmt.Sprintf(
		"curl -L https://github.com/ctsunny/panel/releases/download/1.5.5/install.sh -o ./install.sh && chmod +x ./install.sh && ./install.sh -a %s -s %s",
		serverAddr, node.Secret,
	)
	Rok(c, cmd)
}

// validatePortRange checks that port range is valid
func validatePortRange(sta, end int) error {
	if sta < 1 || sta > 65535 || end < 1 || end > 65535 {
		return fmt.Errorf("端口必须在1-65535范围内")
	}
	if end < sta {
		return fmt.Errorf("结束端口不能小于起始端口")
	}
	return nil
}

// generateSecret generates a cryptographically random node secret
func generateSecret() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: if crypto/rand fails, panic as this is a security-critical operation
		panic("failed to generate secure random bytes: " + err.Error())
	}
	return fmt.Sprintf("%x", b)
}

func processServerAddress(addr string) string {
	if addr == "" {
		return addr
	}
	if addr[0] == '[' {
		return addr
	}

	lastColon := -1
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			lastColon = i
			break
		}
	}

	if lastColon == -1 {
		if isIPv6(addr) {
			return "[" + addr + "]"
		}
		return addr
	}

	host := addr[:lastColon]
	port := addr[lastColon:]
	if isIPv6(host) {
		return "[" + host + "]" + port
	}
	return addr
}

func isIPv6(s string) bool {
	colons := 0
	for _, ch := range s {
		if ch == ':' {
			colons++
		}
	}
	return colons >= 2
}
