package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/bluegitter/docker-mgr/bootstrap"
	"github.com/bluegitter/docker-mgr/routes"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
)

//go:embed templates/*
var templateFS embed.FS

func main() {
	// 解析命令行参数
	portFlag := flag.Int("port", 8000, "The port to listen on")
	flag.Parse()

	bootstrap.Init()

	// 创建Docker客户端
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.41"))
	if err != nil {
		log.Fatalf("Error creating Docker client: %v", err)
		return
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	routes.BuildRoutes(r, templateFS, cli)

	// 运行Web服务器
	port := *portFlag
	printedWarning := false
	for {
		ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
		if err == nil {
			ln.Close()
			break
		}

		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "listen" {
			if !printedWarning {
				fmt.Printf("\033[31mPort %d is already in use, trying next available port.\033[0m\n", port)
				printedWarning = true
			}
		} else {
			fmt.Printf("Error while trying to listen on port %d: %v\n", port, err)
			return
		}
		port++
	}

	fmt.Printf("Listening on port \033[32m%d\033[0m.\n", port)

	// 运行Web服务器
	r.Run(":" + strconv.Itoa(port))
}
