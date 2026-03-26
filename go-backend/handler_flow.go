package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// Flow accounting with fine-grained locking (mimics Java ConcurrentHashMap + synchronized blocks)
var (
	forwardLocks sync.Map
	userLocks    sync.Map
	tunnelLocks  sync.Map
)

func getForwardLock(id string) *sync.Mutex {
	v, _ := forwardLocks.LoadOrStore(id, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func getUserLock(id string) *sync.Mutex {
	v, _ := userLocks.LoadOrStore(id, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func getTunnelLock(id string) *sync.Mutex {
	v, _ := tunnelLocks.LoadOrStore(id, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// HandleFlowUpload processes flow data uploaded by GOST nodes
func HandleFlowUpload(c *gin.Context) {
	secret := c.Query("secret")
	if secret == "" {
		c.String(200, "ok")
		return
	}

	// Validate node
	var nodeCount int64
	DB.Model(&Node{}).Where("secret = ?", secret).Count(&nodeCount)
	if nodeCount == 0 {
		c.String(200, "ok")
		return
	}

	rawData, err := c.GetRawData()
	if err != nil {
		c.String(200, "ok")
		return
	}

	// Decrypt if needed
	decrypted := decryptPayload(string(rawData), secret)

	// Parse flow data
	var flow FlowUploadDTO
	if err := json.Unmarshal([]byte(decrypted), &flow); err != nil {
		c.String(200, "ok")
		return
	}

	// Skip web_api service
	if flow.N == "web_api" {
		c.String(200, "ok")
		return
	}

	log.Printf("Flow upload: %+v", flow)

	processFlowData(&flow, secret)
	c.String(200, "ok")
}

// HandleFlowConfig processes gost config reports from nodes
func HandleFlowConfig(c *gin.Context) {
	secret := c.Query("secret")

	var node Node
	if err := DB.Where("secret = ?", secret).First(&node).Error; err != nil {
		c.String(200, "ok")
		return
	}

	rawData, err := c.GetRawData()
	if err != nil {
		c.String(200, "ok")
		return
	}

	decrypted := decryptPayload(string(rawData), secret)

	var gostCfg GostConfigDTO
	if err := json.Unmarshal([]byte(decrypted), &gostCfg); err != nil {
		c.String(200, "ok")
		return
	}

	// Async cleanup
	go cleanNodeConfigs(node.ID, &gostCfg)

	c.String(200, "ok")
}

func HandleFlowTest(c *gin.Context) {
	c.String(200, "test")
}

func processFlowData(flow *FlowUploadDTO, secret string) {
	parts := strings.Split(flow.N, "_")
	if len(parts) < 3 {
		return
	}
	forwardID := parts[0]
	userID := parts[1]
	userTunnelID := parts[2]

	// Get forward and tunnel for flow type and traffic ratio
	var forward Forward
	if err := DB.Where("id = ?", forwardID).First(&forward).Error; err != nil {
		return
	}

	// Apply traffic ratio and flow type
	var tunnel Tunnel
	if err := DB.First(&tunnel, forward.TunnelID).Error; err == nil {
		d := float64(flow.D) * tunnel.TrafficRatio
		u := float64(flow.U) * tunnel.TrafficRatio
		flowType := tunnel.Flow
		if flowType == 0 {
			flowType = 2 // default bidirectional
		}
		flow.D = int64(d) * int64(flowType)
		flow.U = int64(u) * int64(flowType)
	}

	// Update flows atomically
	updateForwardFlow(forwardID, flow.D, flow.U)
	updateUserFlow(userID, flow.D, flow.U)
	updateUserTunnelFlow(userTunnelID, flow.D, flow.U)

	// Check limits (only for non-admin tunnels)
	if userTunnelID != "0" {
		sn := flow.N
		go checkAndPauseIfNeeded(userID, userTunnelID, sn)
	}
}

func updateForwardFlow(forwardID string, d, u int64) {
	lock := getForwardLock(forwardID)
	lock.Lock()
	defer lock.Unlock()
	DB.Exec("UPDATE forwards SET in_flow = in_flow + ?, out_flow = out_flow + ? WHERE id = ?", d, u, forwardID)
}

func updateUserFlow(userID string, d, u int64) {
	lock := getUserLock(userID)
	lock.Lock()
	defer lock.Unlock()
	DB.Exec("UPDATE users SET in_flow = in_flow + ?, out_flow = out_flow + ? WHERE id = ?", d, u, userID)
}

func updateUserTunnelFlow(userTunnelID string, d, u int64) {
	if userTunnelID == "0" {
		return
	}
	lock := getTunnelLock(userTunnelID)
	lock.Lock()
	defer lock.Unlock()
	DB.Exec("UPDATE user_tunnels SET in_flow = in_flow + ?, out_flow = out_flow + ? WHERE id = ?", d, u, userTunnelID)
}

func checkAndPauseIfNeeded(userID, userTunnelID, _ string) {
	// Re-check user limits
	var user User
	if err := DB.Where("id = ?", userID).First(&user).Error; err != nil {
		return
	}

	needsPause := false
	if user.Flow > 0 {
		limit := user.Flow * 1024 * 1024 * 1024
		if user.InFlow+user.OutFlow >= limit {
			needsPause = true
		}
	}
	if user.ExpTime > 0 && user.ExpTime <= nowMs() {
		needsPause = true
	}
	if user.Status != 1 {
		needsPause = true
	}

	if needsPause {
		pauseAllUserServices(userID)
		return
	}

	// Check user tunnel limits
	var ut UserTunnel
	if err := DB.Where("id = ?", userTunnelID).First(&ut).Error; err != nil {
		return
	}
	utNeedsPause := false
	if ut.Flow > 0 {
		limit := ut.Flow * 1024 * 1024 * 1024
		if ut.InFlow+ut.OutFlow >= limit {
			utNeedsPause = true
		}
	}
	if ut.ExpTime > 0 && ut.ExpTime <= nowMs() {
		utNeedsPause = true
	}
	if ut.Status != 1 {
		utNeedsPause = true
	}

	if utNeedsPause {
		pauseUserTunnelServices(userID, int64(ut.TunnelID))
	}
}

func pauseAllUserServices(userID string) {
	var forwards []Forward
	DB.Where("user_id = ? AND status = 1", userID).Find(&forwards)
	for _, f := range forwards {
		var tunnel Tunnel
		if err := DB.First(&tunnel, f.TunnelID).Error; err != nil {
			continue
		}
		utID := getUserTunnelIDByStr(userID, f.TunnelID)
		sn := ServiceName(f.ID, f.UserID, utID)
		GostPauseService(tunnel.InNodeID, sn)
		if tunnel.Type == 2 {
			GostPauseRemoteService(tunnel.OutNodeID, sn)
		}
		DB.Model(&Forward{}).Where("id = ?", f.ID).Update("status", 0)
	}
}

func pauseUserTunnelServices(userID string, tunnelID int64) {
	var forwards []Forward
	DB.Where("user_id = ? AND tunnel_id = ? AND status = 1", userID, tunnelID).Find(&forwards)
	var tunnel Tunnel
	if err := DB.First(&tunnel, tunnelID).Error; err != nil {
		return
	}
	for _, f := range forwards {
		utID := getUserTunnelIDByStr(userID, tunnelID)
		sn := ServiceName(f.ID, f.UserID, utID)
		GostPauseService(tunnel.InNodeID, sn)
		if tunnel.Type == 2 {
			GostPauseRemoteService(tunnel.OutNodeID, sn)
		}
		DB.Model(&Forward{}).Where("id = ?", f.ID).Update("status", 0)
	}
}

func getUserTunnelIDByStr(userID string, tunnelID int64) int {
	var ut UserTunnel
	if err := DB.Where("user_id = ? AND tunnel_id = ?", userID, tunnelID).First(&ut).Error; err != nil {
		return 0
	}
	return ut.ID
}

func decryptPayload(rawData, secret string) string {
	var em EncryptedMessage
	if err := json.Unmarshal([]byte(rawData), &em); err == nil && em.Encrypted && em.Data != "" {
		crypto := NewAESCrypto(secret)
		if plaintext, err := crypto.Decrypt(em.Data); err == nil {
			return plaintext
		}
	}
	return rawData
}

func cleanNodeConfigs(nodeID int64, cfg *GostConfigDTO) {
	// Clean orphaned services
	for _, svc := range cfg.Services {
		if svc.Name == "web_api" {
			continue
		}
		parts := strings.Split(svc.Name, "_")
		if len(parts) != 4 {
			continue
		}
		forwardID := parts[0]
		userID := parts[1]
		userTunnelID := parts[2]
		typeStr := parts[3]

		baseName := forwardID + "_" + userID + "_" + userTunnelID

		if typeStr == "tcp" {
			var count int64
			DB.Model(&Forward{}).Where("id = ?", forwardID).Count(&count)
			if count == 0 {
				GostDeleteService(nodeID, baseName)
			}
		} else if typeStr == "tls" {
			var count int64
			DB.Model(&Forward{}).Where("id = ?", forwardID).Count(&count)
			if count == 0 {
				GostDeleteRemoteService(nodeID, baseName)
			}
		}
	}

	// Clean orphaned chains
	for _, chain := range cfg.Chains {
		parts := strings.Split(chain.Name, "_")
		if len(parts) != 4 {
			continue
		}
		forwardID := parts[0]
		userID := parts[1]
		userTunnelID := parts[2]
		typeStr := parts[3]
		baseName := forwardID + "_" + userID + "_" + userTunnelID
		if typeStr == "chains" {
			var count int64
			DB.Model(&Forward{}).Where("id = ?", forwardID).Count(&count)
			if count == 0 {
				GostDeleteChains(nodeID, baseName)
			}
		}
	}

	// Clean orphaned limiters
	for _, lim := range cfg.Limiters {
		var count int64
		DB.Model(&SpeedLimit{}).Where("id = ?", lim.Name).Count(&count)
		if count == 0 {
			var limID int64
			if _, err := fmt.Sscanf(lim.Name, "%d", &limID); err == nil {
				GostDeleteLimiters(nodeID, limID)
			}
		}
	}
}
