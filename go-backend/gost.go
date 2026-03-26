package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GostUtil provides helper methods for communicating with GOST nodes

func GostAddLimiters(nodeID, name int64, speed string) GostResult {
	data := createLimiterData(name, speed)
	return Hub.SendMsg(nodeID, data, "AddLimiters")
}

func GostUpdateLimiters(nodeID, name int64, speed string) GostResult {
	req := map[string]interface{}{
		"limiter": fmt.Sprintf("%d", name),
		"data":    createLimiterData(name, speed),
	}
	return Hub.SendMsg(nodeID, req, "UpdateLimiters")
}

func GostDeleteLimiters(nodeID, name int64) GostResult {
	req := map[string]interface{}{
		"limiter": fmt.Sprintf("%d", name),
	}
	return Hub.SendMsg(nodeID, req, "DeleteLimiters")
}

func GostAddService(nodeID int64, name string, inPort int, limiter *int64, remoteAddr string, flowType int, tunnel *Tunnel, strategy string, interfaceName *string) GostResult {
	services := createServiceConfigsFull(name, inPort, limiter, remoteAddr, flowType, tunnel, strategy, interfaceName)
	return Hub.SendMsg(nodeID, services, "AddService")
}

func GostUpdateService(nodeID int64, name string, inPort int, limiter *int64, remoteAddr string, flowType int, tunnel *Tunnel, strategy string, interfaceName *string) GostResult {
	services := createServiceConfigsFull(name, inPort, limiter, remoteAddr, flowType, tunnel, strategy, interfaceName)
	return Hub.SendMsg(nodeID, services, "UpdateService")
}

func GostDeleteService(nodeID int64, name string) GostResult {
	data := map[string]interface{}{
		"services": []string{name + "_tcp", name + "_udp"},
	}
	return Hub.SendMsg(nodeID, data, "DeleteService")
}

func GostAddRemoteService(nodeID int64, name string, outPort int, remoteAddr, protocol, strategy string, interfaceName *string) GostResult {
	data := createRemoteServiceConfig(name, outPort, remoteAddr, protocol, strategy, interfaceName)
	services := []interface{}{data}
	return Hub.SendMsg(nodeID, services, "AddService")
}

func GostUpdateRemoteService(nodeID int64, name string, outPort int, remoteAddr, protocol, strategy string, interfaceName *string) GostResult {
	data := createRemoteServiceConfig(name, outPort, remoteAddr, protocol, strategy, interfaceName)
	services := []interface{}{data}
	return Hub.SendMsg(nodeID, services, "UpdateService")
}

func GostDeleteRemoteService(nodeID int64, name string) GostResult {
	data := map[string]interface{}{
		"services": []string{name + "_tls"},
	}
	return Hub.SendMsg(nodeID, data, "DeleteService")
}

func GostPauseService(nodeID int64, name string) GostResult {
	data := map[string]interface{}{
		"services": []string{name + "_tcp", name + "_udp"},
	}
	return Hub.SendMsg(nodeID, data, "PauseService")
}

func GostResumeService(nodeID int64, name string) GostResult {
	data := map[string]interface{}{
		"services": []string{name + "_tcp", name + "_udp"},
	}
	return Hub.SendMsg(nodeID, data, "ResumeService")
}

func GostPauseRemoteService(nodeID int64, name string) GostResult {
	data := map[string]interface{}{
		"services": []string{name + "_tls"},
	}
	return Hub.SendMsg(nodeID, data, "PauseService")
}

func GostResumeRemoteService(nodeID int64, name string) GostResult {
	data := map[string]interface{}{
		"services": []string{name + "_tls"},
	}
	return Hub.SendMsg(nodeID, data, "ResumeService")
}

func GostAddChains(nodeID int64, name, remoteAddr, protocol string, interfaceName *string) GostResult {
	data := createChainsConfig(name, remoteAddr, protocol, interfaceName)
	return Hub.SendMsg(nodeID, data, "AddChains")
}

func GostUpdateChains(nodeID int64, name, remoteAddr, protocol string, interfaceName *string) GostResult {
	data := createChainsConfig(name, remoteAddr, protocol, interfaceName)
	req := map[string]interface{}{
		"chain": name + "_chains",
		"data":  data,
	}
	return Hub.SendMsg(nodeID, req, "UpdateChains")
}

func GostDeleteChains(nodeID int64, name string) GostResult {
	data := map[string]interface{}{
		"chain": name + "_chains",
	}
	return Hub.SendMsg(nodeID, data, "DeleteChains")
}

func GostSetProtocol(nodeID int64, httpV, tlsV, socksV int) GostResult {
	data := map[string]interface{}{
		"http":  httpV,
		"tls":   tlsV,
		"socks": socksV,
	}
	return Hub.SendMsg(nodeID, data, "SetProtocol")
}

// IsOK returns true if the GOST operation succeeded
func IsOK(r GostResult) bool {
	return r.Msg == "OK"
}

// IsNotFound returns true if the resource was not found
func IsNotFound(r GostResult) bool {
	return strings.Contains(r.Msg, "not found")
}

// ---- helpers ----

func createLimiterData(name int64, speed string) map[string]interface{} {
	return map[string]interface{}{
		"name":   fmt.Sprintf("%d", name),
		"limits": []string{"$ " + speed + "MB " + speed + "MB"},
	}
}

// createServiceConfigsFull creates TCP+UDP service configs for a forward
func createServiceConfigsFull(name string, inPort int, limiter *int64, remoteAddr string, flowType int, tunnel *Tunnel, strategy string, interfaceName *string) []interface{} {
	services := make([]interface{}, 0, 2)
	for _, proto := range []string{"tcp", "udp"} {
		svc := createServiceConfig(name, inPort, limiter, remoteAddr, proto, flowType, tunnel, strategy, interfaceName)
		services = append(services, svc)
	}
	return services
}

func createServiceConfig(name string, inPort int, limiter *int64, remoteAddr, protocol string, flowType int, tunnel *Tunnel, strategy string, interfaceName *string) map[string]interface{} {
	svc := map[string]interface{}{
		"name": name + "_" + protocol,
	}

	if protocol == "tcp" {
		svc["addr"] = tunnel.TCPListenAddr + ":" + fmt.Sprintf("%d", inPort)
	} else {
		svc["addr"] = tunnel.UDPListenAddr + ":" + fmt.Sprintf("%d", inPort)
	}

	if interfaceName != nil && *interfaceName != "" {
		svc["metadata"] = map[string]interface{}{"interface": *interfaceName}
	}

	if limiter != nil {
		svc["limiter"] = fmt.Sprintf("%d", *limiter)
	}

	svc["handler"] = createHandler(protocol, name, flowType)
	svc["listener"] = createListener(protocol)

	if flowType == 1 { // port forward
		svc["forwarder"] = createForwarder(remoteAddr, strategy)
	}

	return svc
}

func createHandler(protocol, name string, flowType int) map[string]interface{} {
	h := map[string]interface{}{"type": protocol}
	if flowType != 1 { // tunnel forward
		h["chain"] = name + "_chains"
	}
	return h
}

func createListener(protocol string) map[string]interface{} {
	l := map[string]interface{}{"type": protocol}
	if protocol == "udp" {
		l["metadata"] = map[string]interface{}{"keepAlive": true}
	}
	return l
}

func createForwarder(remoteAddr, strategy string) map[string]interface{} {
	addrs := strings.Split(remoteAddr, ",")
	nodes := make([]map[string]interface{}, 0, len(addrs))
	for i, addr := range addrs {
		nodes = append(nodes, map[string]interface{}{
			"name": fmt.Sprintf("node_%d", i+1),
			"addr": strings.TrimSpace(addr),
		})
	}

	if strategy == "" {
		strategy = "fifo"
	}

	return map[string]interface{}{
		"nodes": nodes,
		"selector": map[string]interface{}{
			"strategy":    strategy,
			"maxFails":    1,
			"failTimeout": "600s",
		},
	}
}

func createRemoteServiceConfig(name string, outPort int, remoteAddr, protocol, strategy string, interfaceName *string) map[string]interface{} {
	svc := map[string]interface{}{
		"name": name + "_tls",
		"addr": ":" + fmt.Sprintf("%d", outPort),
	}

	if interfaceName != nil && *interfaceName != "" {
		svc["metadata"] = map[string]interface{}{"interface": *interfaceName}
	}

	svc["handler"] = map[string]interface{}{"type": "relay"}
	svc["listener"] = map[string]interface{}{"type": protocol}

	addrs := strings.Split(remoteAddr, ",")
	nodes := make([]map[string]interface{}, 0, len(addrs))
	for i, addr := range addrs {
		nodes = append(nodes, map[string]interface{}{
			"name": fmt.Sprintf("node_%d", i+1),
			"addr": strings.TrimSpace(addr),
		})
	}

	if strategy == "" {
		strategy = "fifo"
	}

	svc["forwarder"] = map[string]interface{}{
		"nodes": nodes,
		"selector": map[string]interface{}{
			"strategy":    strategy,
			"maxFails":    1,
			"failTimeout": "600s",
		},
	}

	return svc
}

func createChainsConfig(name, remoteAddr, protocol string, interfaceName *string) map[string]interface{} {
	dialer := map[string]interface{}{"type": protocol}
	if protocol == "quic" {
		dialer["metadata"] = map[string]interface{}{
			"keepAlive": true,
			"ttl":       "10s",
		}
	}

	node := map[string]interface{}{
		"name":      "node-" + name,
		"addr":      remoteAddr,
		"connector": map[string]interface{}{"type": "relay"},
		"dialer":    dialer,
	}

	if interfaceName != nil && *interfaceName != "" {
		node["interface"] = *interfaceName
	}

	return map[string]interface{}{
		"name": name + "_chains",
		"hops": []interface{}{
			map[string]interface{}{
				"name":  "hop-" + name,
				"nodes": []interface{}{node},
			},
		},
	}
}

// ServiceName builds the GOST service name for a forward
func ServiceName(forwardID, userID int64, userTunnelID int) string {
	return fmt.Sprintf("%d_%d_%d", forwardID, userID, userTunnelID)
}

// rawJSON converts an interface to json.RawMessage
func rawJSON(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
