// File: discovery.go

package discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

// NodeInfo 表示一个节点的信息
type NodeInfo struct {
	Name      string        `json:"name"`      // 节点名称
	IP        string        `json:"ip"`        // IP 地址
	Port      string        `json:"port"`      // 服务端口
	Role      string        `json:"role"`      // 角色标识（如 worker、master）
	Version   string        `json:"version"`   // 版本信息
	LastSeen  time.Time     `json:"-"`         // 最后一次接收到心跳的时间
	Reachable bool          `json:"reachable"` // 是否可达
	Latency   time.Duration `json:"latency"`   // 连通性延迟
}

// Listener 是节点变化监听器接口
// 实现它以接收节点上线和下线事件
type Listener interface {
	OnNodeUpdate(node NodeInfo) // 节点新增或更新
	OnNodeDelete(name string)   // 节点删除
}

// Discovery 是服务发现的核心结构体
// 负责广播、监听、节点管理等
type Discovery struct {
	SelfName string // 当前节点名称
	SelfPort string // 当前节点服务端口
	Role     string // 当前节点角色
	Version  string // 当前节点版本

	nodes     map[string]NodeInfo // 所有发现的节点
	mu        sync.RWMutex        // 读写锁，保护 nodes
	listeners []Listener          // 注册的监听器
}

// NewDiscovery 创建一个服务发现实例
func NewDiscovery(selfName, selfPort, role, version string) *Discovery {
	return &Discovery{
		SelfName:  selfName,
		SelfPort:  selfPort,
		Role:      role,
		Version:   version,
		nodes:     make(map[string]NodeInfo),
		listeners: make([]Listener, 0),
	}
}

// RegisterListener 注册一个监听器接收节点变更通知
func (d *Discovery) RegisterListener(l Listener) {
	d.listeners = append(d.listeners, l)
}

// Start 启动服务发现（广播、监听、清理）
func (d *Discovery) Start() {
	go d.broadcastLoop()
	go d.listenLoop()
	go d.cleanupLoop()
}

// broadcastLoop 定期广播自己的节点信息
func (d *Discovery) broadcastLoop() {
	addr, _ := net.ResolveUDPAddr("udp", "224.0.0.250:11111")
	conn, _ := net.DialUDP("udp", nil, addr)
	defer conn.Close()

	ip := GetLocalIP()

	for {
		msg := map[string]interface{}{
			"name":      d.SelfName,
			"ip":        ip,
			"port":      d.SelfPort,
			"role":      d.Role,
			"version":   d.Version,
			"timestamp": time.Now().UnixMilli(),
		}
		b, _ := json.Marshal(msg)
		conn.Write(b)
		time.Sleep(5 * time.Second)
	}
}

// listenLoop 监听多播地址并处理节点消息
func (d *Discovery) listenLoop() {

	addr, _ := net.ResolveUDPAddr("udp", "224.0.0.250:11111")
	iface := getMulticastInterface()
	conn, err := net.ListenMulticastUDP("udp", iface, addr)
	if err != nil {
		fmt.Println("无法加入多播组:", err)
		os.Exit(1)
	}
	defer conn.Close()
	conn.SetReadBuffer(2048)
	buf := make([]byte, 1024)

	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		var data map[string]interface{}
		dec := json.NewDecoder(bytes.NewReader(buf[:n]))
		if err := dec.Decode(&data); err != nil {
			continue
		}
		name := data["name"].(string)
		if name == d.SelfName {
			continue
		}

		ip := data["ip"].(string)
		port := data["port"].(string)
		role := data["role"].(string)
		version := data["version"].(string)

		reachable, latency := testHTTP(ip, port)

		node := NodeInfo{
			Name: name, IP: ip, Port: port, Role: role, Version: version,
			LastSeen: time.Now(), Reachable: reachable, Latency: latency,
		}

		d.mu.Lock()
		_, existed := d.nodes[name]
		d.nodes[name] = node
		d.mu.Unlock()

		if !existed {
			for _, l := range d.listeners {
				go l.OnNodeUpdate(node)
			}
		}
	}
}
func getMulticastInterface() *net.Interface {
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		// 跳过环回接口和未启用的接口
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		// 使用第一个符合条件的接口
		return &iface
	}
	return nil
}

// cleanupLoop 定期清理长时间未响应的节点
func (d *Discovery) cleanupLoop() {
	for {
		time.Sleep(10 * time.Second)
		now := time.Now()
		d.mu.Lock()
		for name, node := range d.nodes {
			if now.Sub(node.LastSeen) > 15*time.Second {
				delete(d.nodes, name)
				for _, l := range d.listeners {
					go l.OnNodeDelete(name)
				}
			}
		}
		d.mu.Unlock()
	}
}

// AddStaticNode 手动添加一个静态节点
func (d *Discovery) AddStaticNode(ip, port string) {
	reachable, latency := testHTTP(ip, port)
	name := fmt.Sprintf("static-%s:%s", ip, port)
	node := NodeInfo{
		Name: name, IP: ip, Port: port, Role: "static", Version: "manual",
		LastSeen: time.Now(), Reachable: reachable, Latency: latency,
	}
	d.mu.Lock()
	d.nodes[name] = node
	d.mu.Unlock()

	for _, l := range d.listeners {
		go l.OnNodeUpdate(node)
	}
}

// ListNodes 获取所有当前已知节点
func (d *Discovery) ListNodes() []NodeInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()
	list := make([]NodeInfo, 0, len(d.nodes))
	for _, n := range d.nodes {
		list = append(list, n)
	}
	return list
}

// GetLocalIP 获取本机局域网 IP 地址
func GetLocalIP() string {
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String()
		}
	}
	return "127.0.0.1"
}

// testHTTP 测试给定 IP:Port 的 TCP 连通性（非真正 HTTP 请求）
func testHTTP(ip, port string) (bool, time.Duration) {
	client := &net.Dialer{Timeout: 2 * time.Second}
	start := time.Now()
	conn, err := client.Dial("tcp", ip+":"+port)
	if err != nil {
		return false, 0
	}
	latency := time.Since(start)
	conn.Close()
	return true, latency
}
