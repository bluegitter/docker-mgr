package main

import (
	"embed"
	"log"

	"github.com/bluegitter/docker-mgr/routes"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
)

//go:embed templates/*
var templateFS embed.FS

func main() {
	// 创建Docker客户端
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.41"))
	if err != nil {
		log.Fatalf("Error creating Docker client: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	routes.BuildRoutes(r, templateFS, cli)

	// 运行Web服务器
	r.Run(":8080")
}
