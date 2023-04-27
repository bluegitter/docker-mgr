// routes/routes.go
package routes

import (
	"embed"

	"github.com/bluegitter/docker-mgr/handlers"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
)

func BuildRoutes(r *gin.Engine, templateFS embed.FS, cli *client.Client) {
	// 使用处理函数
	r.GET("/", func(c *gin.Context) { handlers.IndexHandler(c, templateFS) })
	// 镜像列表路由
	r.GET("/images", func(c *gin.Context) { handlers.ImagesHandler(c, cli) })
	// 容器列表路由
	r.GET("/containers", func(c *gin.Context) { handlers.ContainersListHandler(c, cli) })
	r.POST("/containers/:id/start", func(c *gin.Context) { handlers.StartContainerHandler(c, cli) })
	r.POST("/containers/:id/stop", func(c *gin.Context) { handlers.StopContainerHandler(c, cli) })
	r.POST("/containers/:id/remove", func(c *gin.Context) { handlers.RemoveContainerHandler(c, cli) })
	r.POST("/create_anaconda_container", func(c *gin.Context) { handlers.CreateAnacondaContainerHandler(c, cli) })
}
