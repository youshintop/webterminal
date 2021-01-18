package main

import (
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/youshintop/webterminal/pkg/terminal"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	"os"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {

	r := gin.Default()

	r.GET("/ws", ShellWs)

	r.Run("0.0.0.0:8080")
}

var wsupgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func ShellWs(c *gin.Context) {
	var err error
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.Error(err)
		return
	}

	cfg := LoadConfig()
	cs, _ := kubernetes.NewForConfig(cfg)
	terminal.WaitForTerminal(conn, cs, cfg)
}

func LoadConfig() *rest.Config {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		panic("kubeconfig path not found.")
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic("Failed load kubeconfig.")
	}
	return config
}
