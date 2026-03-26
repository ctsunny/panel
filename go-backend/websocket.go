package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true // allow all origins
	},
}

// WSHub manages WebSocket connections from GOST nodes and admin browsers
type WSHub struct {
	// Node connections: nodeID -> *NodeConn
	nodeSessions sync.Map
	// Admin browser sessions
	adminSessions sync.Map
	// Pending requests: requestID -> chan WSResponse
	pendingRequests sync.Map
	// Crypto cache: secret -> *AESCrypto
	cryptoCache sync.Map
}

var Hub = &WSHub{}

type NodeConn struct {
	mu     sync.Mutex
	conn   *websocket.Conn
	nodeID int64
	secret string
}

type WSResponse struct {
	Msg  string          `json:"message"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type WSRequest struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	RequestID string      `json:"requestId"`
}

type GostResult struct {
	Msg  string
	Data json.RawMessage
}

// HandleWebSocket is the Gin handler for /system-info WebSocket endpoint
func HandleWebSocket(c *gin.Context) {
	connType := c.Query("type")
	secret := c.Query("secret")
	version := c.Query("version")
	httpStr := c.Query("http")
	tlsStr := c.Query("tls")
	socksStr := c.Query("socks")

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	if connType == "1" {
		// GOST node connection
		var node Node
		if err := DB.Where("secret = ?", secret).First(&node).Error; err != nil {
			log.Printf("Node authentication failed for secret: %s", secret)
			conn.Close()
			return
		}

		nodeConn := &NodeConn{
			conn:   conn,
			nodeID: node.ID,
			secret: secret,
		}

		// Close any existing session for this node
		if old, ok := Hub.nodeSessions.Load(node.ID); ok {
			oldConn := old.(*NodeConn)
			oldConn.mu.Lock()
			oldConn.conn.Close()
			oldConn.mu.Unlock()
		}
		Hub.nodeSessions.Store(node.ID, nodeConn)

		// Update node status and metadata
		updates := map[string]interface{}{
			"status": 1,
		}
		if version != "" {
			updates["version"] = version
		}
		if httpStr != "" {
			updates["http"] = atoi(httpStr)
		}
		if tlsStr != "" {
			updates["tls"] = atoi(tlsStr)
		}
		if socksStr != "" {
			updates["socks"] = atoi(socksStr)
		}
		DB.Model(&Node{}).Where("id = ?", node.ID).Updates(updates)

		// Broadcast online status to admins
		Hub.BroadcastToAdmins(map[string]interface{}{
			"id":   fmt.Sprintf("%d", node.ID),
			"type": "status",
			"data": 1,
		})

		log.Printf("Node %d connected (version: %s)", node.ID, version)

		go Hub.handleNodeMessages(nodeConn)

	} else {
		// Admin browser connection - validate JWT
		if !ValidateToken(secret) {
			conn.Close()
			return
		}

		userID, _ := GetUserIDFromToken(secret)
		connKey := fmt.Sprintf("admin_%d_%p", userID, conn)
		Hub.adminSessions.Store(connKey, conn)

		log.Printf("Admin %d connected via WebSocket", userID)

		go Hub.handleAdminMessages(conn, connKey)
	}
}

func (h *WSHub) handleNodeMessages(nc *NodeConn) {
	defer func() {
		nc.conn.Close()
		// Only update offline if this is still the current session
		if current, ok := h.nodeSessions.Load(nc.nodeID); ok {
			if current.(*NodeConn) == nc {
				h.nodeSessions.Delete(nc.nodeID)
				DB.Model(&Node{}).Where("id = ?", nc.nodeID).Update("status", 0)
				h.BroadcastToAdmins(map[string]interface{}{
					"id":   fmt.Sprintf("%d", nc.nodeID),
					"type": "status",
					"data": 0,
				})
				log.Printf("Node %d disconnected", nc.nodeID)
			}
		}
	}()

	for {
		_, msgBytes, err := nc.conn.ReadMessage()
		if err != nil {
			break
		}

		// Decrypt if needed
		msg := h.decryptIfNeeded(string(msgBytes), nc.secret)

		// Check if it's a heartbeat / memory_usage message
		if containsStr(msg, "memory_usage") {
			_ = h.sendRaw(nc, `{"type":"call"}`)
			continue
		}

		// Parse as response to a pending request
		var resp struct {
			RequestID string          `json:"requestId"`
			Message   string          `json:"message"`
			Type      string          `json:"type"`
			Data      json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(msg), &resp); err == nil && resp.RequestID != "" {
			if ch, ok := h.pendingRequests.LoadAndDelete(resp.RequestID); ok {
				result := GostResult{
					Msg:  resp.Message,
					Data: resp.Data,
				}
				if result.Msg == "" {
					result.Msg = "无响应消息"
				}
				ch.(chan GostResult) <- result
			}
		}

	// Broadcast to admins (for node info display)
	h.BroadcastToAdmins(map[string]interface{}{
		"id":   fmt.Sprintf("%d", nc.nodeID),
		"type": "info",
		"data": msg,
	})
	}
}

func (h *WSHub) handleAdminMessages(conn *websocket.Conn, key string) {
	defer func() {
		conn.Close()
		h.adminSessions.Delete(key)
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// SendMsg sends a command to a GOST node and waits for the response (10s timeout)
func (h *WSHub) SendMsg(nodeID int64, data interface{}, msgType string) GostResult {
	nc, ok := h.nodeSessions.Load(nodeID)
	if !ok {
		return GostResult{Msg: "节点不在线"}
	}
	nodeConn := nc.(*NodeConn)

	requestID := uuid.New().String()
	ch := make(chan GostResult, 1)
	h.pendingRequests.Store(requestID, ch)

	req := WSRequest{
		Type:      msgType,
		Data:      data,
		RequestID: requestID,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		h.pendingRequests.Delete(requestID)
		return GostResult{Msg: "序列化失败"}
	}

	// Encrypt if crypto available
	msgToSend := h.encryptIfPossible(string(reqJSON), nodeConn.secret)
	if err := h.sendRaw(nodeConn, msgToSend); err != nil {
		h.pendingRequests.Delete(requestID)
		return GostResult{Msg: "发送失败: " + err.Error()}
	}

	select {
	case result := <-ch:
		return result
	case <-time.After(10 * time.Second):
		h.pendingRequests.Delete(requestID)
		return GostResult{Msg: "等待响应超时"}
	}
}

// BroadcastToAdmins broadcasts a message to all connected admin browsers
func (h *WSHub) BroadcastToAdmins(data interface{}) {
	msgBytes, err := json.Marshal(data)
	if err != nil {
		return
	}
	var failedKeys []interface{}
	h.adminSessions.Range(func(key, value interface{}) bool {
		conn := value.(*websocket.Conn)
		if err := conn.WriteMessage(websocket.TextMessage, msgBytes); err != nil {
			failedKeys = append(failedKeys, key)
		}
		return true
	})
	for _, k := range failedKeys {
		h.adminSessions.Delete(k)
	}
}

func (h *WSHub) sendRaw(nc *NodeConn, msg string) error {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	return nc.conn.WriteMessage(websocket.TextMessage, []byte(msg))
}

func (h *WSHub) decryptIfNeeded(payload, secret string) string {
	var em EncryptedMessage
	if err := json.Unmarshal([]byte(payload), &em); err == nil && em.Encrypted && em.Data != "" {
		crypto := h.getCrypto(secret)
		if crypto != nil {
			if plaintext, err := crypto.Decrypt(em.Data); err == nil {
				return plaintext
			}
		}
	}
	return payload
}

func (h *WSHub) encryptIfPossible(msg, secret string) string {
	if secret == "" {
		return msg
	}
	crypto := h.getCrypto(secret)
	if crypto == nil {
		return msg
	}
	encrypted, err := crypto.Encrypt(msg)
	if err != nil {
		return msg
	}
	em := EncryptedMessage{
		Encrypted: true,
		Data:      encrypted,
		Timestamp: time.Now().UnixMilli(),
	}
	emBytes, err := json.Marshal(em)
	if err != nil {
		return msg
	}
	return string(emBytes)
}

func (h *WSHub) getCrypto(secret string) *AESCrypto {
	if v, ok := h.cryptoCache.Load(secret); ok {
		return v.(*AESCrypto)
	}
	crypto := NewAESCrypto(secret)
	h.cryptoCache.Store(secret, crypto)
	return crypto
}

func atoi(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
