package discovery

import (
	"encoding/json"
	"net/http"
)

// StartRESTServer 启动本地 REST 接口，提供节点状态查询
func (d *Discovery) StartRESTServer(port string) {
	http.HandleFunc("/nodes", func(w http.ResponseWriter, r *http.Request) {
		nodes := d.ListNodes()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodes)
	})

	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	})

	go func() {
		http.ListenAndServe(":"+port, nil)
	}()
}
