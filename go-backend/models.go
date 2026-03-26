package main

import "time"

// User represents a system user
type User struct {
	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	User          string `gorm:"column:user;size:100;not null" json:"user"`
	Pwd           string `gorm:"column:pwd;size:100;not null" json:"pwd,omitempty"`
	RoleID        int    `gorm:"column:role_id;not null" json:"role_id"`
	ExpTime       int64  `gorm:"column:exp_time;not null" json:"exp_time"`
	Flow          int64  `gorm:"column:flow;not null" json:"flow"`
	InFlow        int64  `gorm:"column:in_flow;not null;default:0" json:"in_flow"`
	OutFlow       int64  `gorm:"column:out_flow;not null;default:0" json:"out_flow"`
	FlowResetTime int64  `gorm:"column:flow_reset_time;not null" json:"flow_reset_time"`
	Num           int    `gorm:"column:num;not null" json:"num"`
	CreatedTime   int64  `gorm:"column:created_time;not null" json:"created_time"`
	UpdatedTime   *int64 `gorm:"column:updated_time" json:"updated_time"`
	Status        int    `gorm:"column:status;not null" json:"status"`
}

// Node represents a GOST proxy node
type Node struct {
	ID          int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string  `gorm:"column:name;size:100;not null" json:"name"`
	Secret      string  `gorm:"column:secret;size:100;not null" json:"secret"`
	IP          *string `gorm:"column:ip;type:text" json:"ip"`
	ServerIP    string  `gorm:"column:server_ip;size:100;not null" json:"server_ip"`
	PortSta     int     `gorm:"column:port_sta;not null" json:"port_sta"`
	PortEnd     int     `gorm:"column:port_end;not null" json:"port_end"`
	Version     *string `gorm:"column:version;size:100" json:"version"`
	HTTP        int     `gorm:"column:http;not null;default:0" json:"http"`
	TLS         int     `gorm:"column:tls;not null;default:0" json:"tls"`
	Socks       int     `gorm:"column:socks;not null;default:0" json:"socks"`
	CreatedTime int64   `gorm:"column:created_time;not null" json:"created_time"`
	UpdatedTime *int64  `gorm:"column:updated_time" json:"updated_time"`
	Status      int     `gorm:"column:status;not null" json:"status"`
}

// Tunnel represents a GOST tunnel
type Tunnel struct {
	ID            int64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string   `gorm:"column:name;size:100;not null" json:"name"`
	TrafficRatio  float64  `gorm:"column:traffic_ratio;not null;default:1.0" json:"traffic_ratio"`
	InNodeID      int64    `gorm:"column:in_node_id;not null" json:"in_node_id"`
	InIP          string   `gorm:"column:in_ip;size:100;not null" json:"in_ip"`
	OutNodeID     int64    `gorm:"column:out_node_id;not null" json:"out_node_id"`
	OutIP         string   `gorm:"column:out_ip;size:100;not null" json:"out_ip"`
	Type          int      `gorm:"column:type;not null" json:"type"`
	Protocol      *string  `gorm:"column:protocol;size:10" json:"protocol"`
	Flow          int      `gorm:"column:flow;not null" json:"flow"`
	TCPListenAddr string   `gorm:"column:tcp_listen_addr;size:100;not null;default:'[::]'" json:"tcp_listen_addr"`
	UDPListenAddr string   `gorm:"column:udp_listen_addr;size:100;not null;default:'[::]'" json:"udp_listen_addr"`
	InterfaceName *string  `gorm:"column:interface_name;size:200" json:"interface_name"`
	CreatedTime   int64    `gorm:"column:created_time;not null" json:"created_time"`
	UpdatedTime   int64    `gorm:"column:updated_time;not null" json:"updated_time"`
	Status        int      `gorm:"column:status;not null" json:"status"`
}

// Forward represents a port forward rule
type Forward struct {
	ID            int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        int64   `gorm:"column:user_id;not null" json:"user_id"`
	UserName      string  `gorm:"column:user_name;size:100;not null" json:"user_name"`
	Name          string  `gorm:"column:name;size:100;not null" json:"name"`
	TunnelID      int64   `gorm:"column:tunnel_id;not null" json:"tunnel_id"`
	InPort        int     `gorm:"column:in_port;not null" json:"in_port"`
	OutPort       *int    `gorm:"column:out_port" json:"out_port"`
	RemoteAddr    string  `gorm:"column:remote_addr;type:text;not null" json:"remote_addr"`
	Strategy      string  `gorm:"column:strategy;size:100;not null;default:'fifo'" json:"strategy"`
	InterfaceName *string `gorm:"column:interface_name;size:200" json:"interface_name"`
	InFlow        int64   `gorm:"column:in_flow;not null;default:0" json:"in_flow"`
	OutFlow       int64   `gorm:"column:out_flow;not null;default:0" json:"out_flow"`
	CreatedTime   int64   `gorm:"column:created_time;not null" json:"created_time"`
	UpdatedTime   int64   `gorm:"column:updated_time;not null" json:"updated_time"`
	Status        int     `gorm:"column:status;not null" json:"status"`
	Inx           int     `gorm:"column:inx;not null;default:0" json:"inx"`
}

// UserTunnel represents a user's permission for a tunnel
type UserTunnel struct {
	ID            int    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        int64  `gorm:"column:user_id;not null" json:"user_id"`
	TunnelID      int64  `gorm:"column:tunnel_id;not null" json:"tunnel_id"`
	SpeedID       *int64 `gorm:"column:speed_id" json:"speed_id"`
	Num           int    `gorm:"column:num;not null" json:"num"`
	Flow          int64  `gorm:"column:flow;not null" json:"flow"`
	InFlow        int64  `gorm:"column:in_flow;not null;default:0" json:"in_flow"`
	OutFlow       int64  `gorm:"column:out_flow;not null;default:0" json:"out_flow"`
	FlowResetTime int64  `gorm:"column:flow_reset_time;not null" json:"flow_reset_time"`
	ExpTime       int64  `gorm:"column:exp_time;not null" json:"exp_time"`
	Status        int    `gorm:"column:status;not null" json:"status"`
}

// SpeedLimit represents a speed limit rule
type SpeedLimit struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string `gorm:"column:name;size:100;not null" json:"name"`
	Speed       int    `gorm:"column:speed;not null" json:"speed"`
	TunnelID    int64  `gorm:"column:tunnel_id;not null" json:"tunnel_id"`
	TunnelName  string `gorm:"column:tunnel_name;size:100;not null" json:"tunnel_name"`
	CreatedTime int64  `gorm:"column:created_time;not null" json:"created_time"`
	UpdatedTime *int64 `gorm:"column:updated_time" json:"updated_time"`
	Status      int    `gorm:"column:status;not null" json:"status"`
}

// StatisticsFlow represents hourly flow statistics per user
type StatisticsFlow struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      int64  `gorm:"column:user_id;not null" json:"user_id"`
	Flow        int64  `gorm:"column:flow;not null" json:"flow"`
	TotalFlow   int64  `gorm:"column:total_flow;not null" json:"total_flow"`
	Time        string `gorm:"column:time;size:100;not null" json:"time"`
	CreatedTime int64  `gorm:"column:created_time;not null" json:"created_time"`
}

// ViteConfig represents frontend configuration
type ViteConfig struct {
	ID    int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name  string `gorm:"column:name;size:200;not null;uniqueIndex" json:"name"`
	Value string `gorm:"column:value;size:200;not null" json:"value"`
	Time  int64  `gorm:"column:time;not null" json:"time"`
}

// -- DTOs --

type LoginDTO struct {
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
	CaptchaID string `json:"captchaId"`
}

type CreateUserDTO struct {
	User          string `json:"user" binding:"required"`
	Pwd           string `json:"pwd" binding:"required"`
	RoleID        int    `json:"role_id"`
	ExpTime       int64  `json:"exp_time"`
	Flow          int64  `json:"flow"`
	FlowResetTime int64  `json:"flow_reset_time"`
	Num           int    `json:"num"`
	Status        int    `json:"status"`
}

type UpdateUserDTO struct {
	ID            int64  `json:"id" binding:"required"`
	User          string `json:"user"`
	Pwd           string `json:"pwd"`
	ExpTime       int64  `json:"exp_time"`
	Flow          int64  `json:"flow"`
	FlowResetTime int64  `json:"flow_reset_time"`
	Num           int    `json:"num"`
	Status        int    `json:"status"`
}

type ChangePasswordDTO struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required"`
	ConfirmPwd  string `json:"confirmPwd" binding:"required"`
}

type ResetFlowDTO struct {
	ID int64 `json:"id" binding:"required"`
}

type CreateNodeDTO struct {
	Name     string `json:"name" binding:"required"`
	IP       string `json:"ip" binding:"required"`
	ServerIP string `json:"server_ip" binding:"required"`
	PortSta  int    `json:"port_sta" binding:"required"`
	PortEnd  int    `json:"port_end" binding:"required"`
}

type UpdateNodeDTO struct {
	ID       int64  `json:"id" binding:"required"`
	Name     string `json:"name"`
	IP       string `json:"ip"`
	ServerIP string `json:"server_ip"`
	PortSta  int    `json:"port_sta"`
	PortEnd  int    `json:"port_end"`
	HTTP     *int   `json:"http"`
	TLS      *int   `json:"tls"`
	Socks    *int   `json:"socks"`
}

type CreateTunnelDTO struct {
	Name          string   `json:"name" binding:"required"`
	InNodeID      int64    `json:"in_node_id" binding:"required"`
	OutNodeID     *int64   `json:"out_node_id"`
	Type          int      `json:"type" binding:"required"`
	Protocol      string   `json:"protocol"`
	Flow          int      `json:"flow"`
	TrafficRatio  float64  `json:"traffic_ratio"`
	TCPListenAddr string   `json:"tcp_listen_addr"`
	UDPListenAddr string   `json:"udp_listen_addr"`
	InterfaceName *string  `json:"interface_name"`
}

type UpdateTunnelDTO struct {
	ID            int64    `json:"id" binding:"required"`
	Name          string   `json:"name"`
	Flow          int      `json:"flow"`
	TrafficRatio  float64  `json:"traffic_ratio"`
	TCPListenAddr string   `json:"tcp_listen_addr"`
	UDPListenAddr string   `json:"udp_listen_addr"`
	Protocol      string   `json:"protocol"`
	InterfaceName *string  `json:"interface_name"`
	Status        int      `json:"status"`
}

type CreateForwardDTO struct {
	TunnelID      int64   `json:"tunnel_id" binding:"required"`
	Name          string  `json:"name" binding:"required"`
	InPort        *int    `json:"in_port"`
	OutPort       *int    `json:"out_port"`
	RemoteAddr    string  `json:"remote_addr"`
	Strategy      string  `json:"strategy"`
	InterfaceName *string `json:"interface_name"`
}

type UpdateForwardDTO struct {
	ID            int64   `json:"id" binding:"required"`
	UserID        int64   `json:"user_id"`
	TunnelID      int64   `json:"tunnel_id"`
	Name          string  `json:"name"`
	InPort        *int    `json:"in_port"`
	OutPort       *int    `json:"out_port"`
	RemoteAddr    string  `json:"remote_addr"`
	Strategy      string  `json:"strategy"`
	InterfaceName *string `json:"interface_name"`
}

type AssignUserTunnelDTO struct {
	UserID        int64  `json:"user_id" binding:"required"`
	TunnelID      int64  `json:"tunnel_id" binding:"required"`
	Flow          int64  `json:"flow"`
	Num           int    `json:"num"`
	FlowResetTime int64  `json:"flow_reset_time"`
	ExpTime       int64  `json:"exp_time"`
	SpeedID       *int64 `json:"speed_id"`
	Status        int    `json:"status"`
}

type UpdateUserTunnelDTO struct {
	ID            int    `json:"id" binding:"required"`
	Flow          int64  `json:"flow"`
	Num           int    `json:"num"`
	FlowResetTime *int64 `json:"flow_reset_time"`
	ExpTime       *int64 `json:"exp_time"`
	SpeedID       *int64 `json:"speed_id"`
	Status        *int   `json:"status"`
}

type CreateSpeedLimitDTO struct {
	Name       string `json:"name" binding:"required"`
	Speed      int    `json:"speed" binding:"required"`
	TunnelID   int64  `json:"tunnel_id" binding:"required"`
	TunnelName string `json:"tunnel_name" binding:"required"`
}

type UpdateSpeedLimitDTO struct {
	ID         int64  `json:"id" binding:"required"`
	Name       string `json:"name"`
	Speed      int    `json:"speed"`
	TunnelID   int64  `json:"tunnel_id"`
	TunnelName string `json:"tunnel_name"`
}

// ForwardWithTunnelDTO is returned by forward list queries
type ForwardWithTunnelDTO struct {
	ID            int64   `json:"id"`
	UserID        int64   `json:"user_id"`
	UserName      string  `json:"user_name"`
	Name          string  `json:"name"`
	TunnelID      int64   `json:"tunnel_id"`
	TunnelName    string  `json:"tunnel_name"`
	InPort        int     `json:"in_port"`
	OutPort       *int    `json:"out_port"`
	RemoteAddr    string  `json:"remote_addr"`
	Strategy      string  `json:"strategy"`
	InterfaceName *string `json:"interface_name"`
	InFlow        int64   `json:"in_flow"`
	OutFlow       int64   `json:"out_flow"`
	InIP          string  `json:"in_ip"`
	OutIP         string  `json:"out_ip"`
	Type          int     `json:"type"`
	Protocol      *string `json:"protocol"`
	Status        int     `json:"status"`
	CreatedTime   int64   `json:"created_time"`
	UpdatedTime   int64   `json:"updated_time"`
	Inx           int     `json:"inx"`
}

// UserTunnelWithDetailDTO is returned by user tunnel list queries
type UserTunnelWithDetailDTO struct {
	ID            int     `json:"id"`
	UserID        int64   `json:"user_id"`
	TunnelID      int64   `json:"tunnel_id"`
	TunnelName    string  `json:"tunnel_name"`
	SpeedID       *int64  `json:"speed_id"`
	Num           int     `json:"num"`
	Flow          int64   `json:"flow"`
	InFlow        int64   `json:"in_flow"`
	OutFlow       int64   `json:"out_flow"`
	FlowResetTime int64   `json:"flow_reset_time"`
	ExpTime       int64   `json:"exp_time"`
	Status        int     `json:"status"`
}

// TunnelListDTO is returned for user tunnel selection
type TunnelListDTO struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	Type          int     `json:"type"`
	Protocol      *string `json:"protocol"`
	Flow          int     `json:"flow"`
	TCPListenAddr string  `json:"tcp_listen_addr"`
	UDPListenAddr string  `json:"udp_listen_addr"`
	InNodeID      int64   `json:"in_node_id"`
	OutNodeID     int64   `json:"out_node_id"`
	Status        int     `json:"status"`
}

// FlowUploadDTO is the flow report from GOST nodes
type FlowUploadDTO struct {
	N string `json:"n"` // service name: forwardId_userId_userTunnelId
	D int64  `json:"d"` // download (in) bytes
	U int64  `json:"u"` // upload (out) bytes
}

// EncryptedMessage wraps encrypted payloads from nodes
type EncryptedMessage struct {
	Encrypted bool   `json:"encrypted"`
	Data      string `json:"data"`
	Timestamp int64  `json:"timestamp"`
}

// GostConfigDTO is the config report from GOST nodes
type GostConfigDTO struct {
	Services []ConfigItem `json:"services"`
	Chains   []ConfigItem `json:"chains"`
	Limiters []ConfigItem `json:"limiters"`
}

type ConfigItem struct {
	Name string `json:"name"`
}

func nowMs() int64 {
	return time.Now().UnixMilli()
}
