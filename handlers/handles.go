// handlers/handlers.go
package handlers

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/bluegitter/docker-mgr/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
)

var JWT_KEY = "wyjXpkrtnV6TRQVuWZeLJWhnwOzNJb5wY6HxyemJY+o="

func LoginHandler(c *gin.Context, templateFS embed.FS) {
	file, err := templateFS.Open("templates/login.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "Error reading login.html")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error reading login.html")
		return
	}

	reader := bytes.NewReader(content)
	http.ServeContent(c.Writer, c.Request, "login.html", time.Now(), reader)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func CheckLoginHandler(c *gin.Context, templateFS embed.FS) {
	var req loginRequest

	// 解析请求体
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// 检查用户名和密码
	if req.Username == config.Username && req.Password == config.Password {
		// 创建 JWT 令牌
		token := jwt.New(jwt.SigningMethodHS256)
		claims := token.Claims.(jwt.MapClaims)
		claims["username"] = req.Username
		claims["exp"] = time.Now().Add(time.Hour * 24).Unix()

		// 签名令牌
		tokenString, err := token.SignedString([]byte(JWT_KEY))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error signing the token"})
			return
		}

		// 返回令牌
		c.JSON(http.StatusOK, gin.H{"token": tokenString})
	} else {
		// 返回错误消息
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
	}
}

func CheckTokenHandler(c *gin.Context) {
	authorizationHeader := c.Request.Header.Get("Authorization")
	if authorizationHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing Authorization header"})
		return
	}

	// Extract the token from the header value
	tokenString := strings.TrimPrefix(authorizationHeader, "Bearer ")
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(JWT_KEY), nil
	})

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}

	if !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Token is valid"})
}

func IndexHandler(c *gin.Context, templateFS embed.FS) {
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
}

func ImagesHandler(c *gin.Context, cli *client.Client) {
	images, err := getImages(cli)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, images)
}

type ContainerWithStats struct {
	Id          string
	CPUUsage    float64
	MemoryUsage uint64
	MemoryLimit uint64
}

func ContainerStatsHandler(c *gin.Context, cli *client.Client) {
	containerID := c.Param("id")
	statsReader, err := cli.ContainerStats(context.Background(), containerID, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var containerStats types.StatsJSON
	err = json.NewDecoder(statsReader.Body).Decode(&containerStats)

	cpuUsage := calculateCPUPercentage(&containerStats)
	memoryUsage := containerStats.MemoryStats.Usage
	memoryLimit := containerStats.MemoryStats.Limit

	var containerWithStats = ContainerWithStats{
		Id:          containerID,
		CPUUsage:    cpuUsage,
		MemoryUsage: memoryUsage,
		MemoryLimit: memoryLimit,
	}

	c.JSON(http.StatusOK, containerWithStats)
}

func ContainersListHandler(c *gin.Context, cli *client.Client) {
	containers, err := getContainers(cli)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, containers)
}

func calculateCPUPercentage(stats *types.StatsJSON) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage) - float64(stats.PreCPUStats.SystemUsage)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		return (cpuDelta / systemDelta) * float64(len(stats.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}

	return 0.0
}

func StartContainerHandler(c *gin.Context, cli *client.Client) {
	containerID := c.Param("id")
	err := startContainer(cli, containerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Container started"})
}

func StopContainerHandler(c *gin.Context, cli *client.Client) {
	containerID := c.Param("id")
	err := stopContainer(cli, containerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Container stopped"})
}

func RemoveContainerHandler(c *gin.Context, cli *client.Client) {
	containerID := c.Param("id")
	err := removeContainer(cli, containerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Container removed successfully"})
}

func CreateAnacondaContainerHandler(c *gin.Context, cli *client.Client) {
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
