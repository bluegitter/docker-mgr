package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
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

	r := gin.Default()
	// Use custom template engine with embedded templates
	r.SetHTMLTemplate(loadTemplatesFromFS(templateFS, "templates"))

	r.GET("/", func(c *gin.Context) {
		file, err := templateFS.Open("templates/index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "Error reading index.html")
			return
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error reading index.html")
			return
		}

		reader := bytes.NewReader(content)
		http.ServeContent(c.Writer, c.Request, "index.html", time.Now(), reader)
	})

	// 镜像列表路由
	r.GET("/images", func(c *gin.Context) {
		images, err := getImages(cli)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, images)
	})

	// 容器列表路由
	r.GET("/containers", func(c *gin.Context) {
		containers, err := getContainers(cli)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, containers)
	})

	r.POST("/containers/:id/start", func(c *gin.Context) {
		containerID := c.Param("id")
		err := startContainer(cli, containerID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Container started"})
	})

	r.POST("/containers/:id/stop", func(c *gin.Context) {
		containerID := c.Param("id")
		err := stopContainer(cli, containerID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Container stopped"})
	})

	r.POST("/containers/:id/remove", func(c *gin.Context) {
		containerID := c.Param("id")
		err := removeContainer(cli, containerID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Container removed successfully"})
	})

	r.POST("/create_anaconda_container", func(c *gin.Context) {
		jupyterPortStr := c.PostForm("jupyter_port")
		sshPortStr := c.PostForm("ssh_port")

		jupyterPort, err := strconv.Atoi(jupyterPortStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Jupyter port"})
			return
		}

		sshPort, err := strconv.Atoi(sshPortStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid SSH port"})
			return
		}

		err = runAnacondaContainer(cli, jupyterPort, sshPort)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Anaconda container created"})
	})

	// 运行Web服务器
	r.Run(":8080")
}

func getImages(cli *client.Client) ([]types.ImageSummary, error) {
	images, err := cli.ImageList(context.Background(), types.ImageListOptions{})
	if err != nil {
		return nil, err
	}
	return images, nil
}

func getContainers(cli *client.Client) ([]types.Container, error) {
	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func startContainer(cli *client.Client, containerID string) error {
	return cli.ContainerStart(context.Background(), containerID, types.ContainerStartOptions{})
}

func stopContainer(cli *client.Client, containerID string) error {
	stopOptions := container.StopOptions{}
	return cli.ContainerStop(context.Background(), containerID, stopOptions)
}

func removeContainer(cli *client.Client, containerID string) error {
	removeOptions := types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	}
	err := cli.ContainerRemove(context.Background(), containerID, removeOptions)
	if err != nil {
		return err
	}

	return nil
}

func loadTemplatesFromFS(fs embed.FS, dir string) *template.Template {
	tmpl := template.New("")
	entries, _ := fs.ReadDir(dir)

	for _, entry := range entries {
		file, _ := fs.Open(dir + "/" + entry.Name())
		content, _ := io.ReadAll(file)
		_, _ = tmpl.New(entry.Name()).Parse(string(content))
	}

	return tmpl
}

func isPortAvailable(port int) bool {
	address := fmt.Sprintf(":%d", port)
	conn, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func runAnacondaContainer(cli *client.Client, jupyterPort, sshPort int) error {
	if !isPortAvailable(jupyterPort) {
		return fmt.Errorf("Jupyter port %d is not available", jupyterPort)
	}

	if !isPortAvailable(sshPort) {
		return fmt.Errorf("SSH port %d is not available", sshPort)
	}

	ctx := context.Background()

	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			"8888/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", jupyterPort)}},
			"22/tcp":   []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", sshPort)}},
		},
		// Binds: []string{"/opt/notebook/workspace:/workspace"},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: "/opt/notebook/workspace",
				Target: "/workspace",
			},
		},
		Runtime: "nvidia",
	}

	config := &container.Config{
		Image: "yanfei/anaconda3:latest",
		Cmd: []string{
			"/bin/bash",
			"-c",
			"/usr/sbin/sshd && mkdir -p /workspace && jupyter notebook --NotebookApp.password='sha1:77b5117ca0a9:f62234b17bee56b22db9d5d2b307b7c42573569f' --notebook-dir=/workspace --ip='*' --port=8888 --no-browser --allow-root",
		},
		ExposedPorts: nat.PortSet{
			"8888/tcp": struct{}{},
			"22/tcp":   struct{}{},
		},
	}

	networkingConfig := &network.NetworkingConfig{}

	resp, err := cli.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, "")
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	fmt.Printf("Anaconda container started with ID: %s\n", resp.ID)
	return nil
}