package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"WukongMesh/discovery"
)

// MyListener 实现 Listener 接口，打印事件
type MyListener struct{}

func (l *MyListener) OnNodeUpdate(node discovery.NodeInfo) {
	fmt.Printf("[节点上线] %s (%s:%s)\n", node.Name, node.IP, node.Port)
}

func (l *MyListener) OnNodeDelete(name string) {
	fmt.Printf("[节点下线] %s\n", name)
}

func main() {

	localIP := discovery.GetLocalIP()
	name := fmt.Sprintf("node-%s", localIP)
	d := discovery.NewDiscovery(name, "11110", "worker", "v1.0")
	d.RegisterListener(&MyListener{})
	d.Start()
	d.StartRESTServer("18080")

	fmt.Println("服务发现启动，REST API 监听在 :18080")

	// 示例：手动添加一个静态节点
	time.Sleep(3 * time.Second)
	// d.AddStaticNode("192.168.1.100", "11110")

	// 阻塞直到退出
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
