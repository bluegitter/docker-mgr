package main

import (
	"embed"
	"flag"
	"log"
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

	port := bootstrap.EnsurePortNotUsed(portFlag)

	// 运行Web服务器
	r.Run(":" + strconv.Itoa(port))
}
