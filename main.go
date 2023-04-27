package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/bluegitter/docker-mgr/routes"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
)

//go:embed templates/*
var templateFS embed.FS

func getLockFilePath() string {
	tmpDir := os.TempDir()
	return filepath.Join(tmpDir, "app.lock")
}

var (
	once     sync.Once
	shutdown context.Context
	cancel   context.CancelFunc
)

// runShutdownHook 执行shutdown hook
func runShutdownHook(f *os.File, lockFile string) {
	once.Do(func() {
		cancel()

		fmt.Println("\nReceived an interrupt, cleaning up...")

		// 在这里执行清理操作，例如删除lockfile
		f.Close()
		os.Remove(lockFile)

		// 退出程序
		os.Exit(0)
	})
}

func main() {
	// 解析命令行参数
	portFlag := flag.Int("port", 8080, "The port to listen on")
	flag.Parse()

	lockFile := getLockFilePath()
	fmt.Printf("lockFile %s.\n", lockFile)

	// 尝试创建一个锁文件
	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			exePath, err := os.Executable()
			if err != nil {
				fmt.Println("无法获取可执行文件路径:", err)
				return
			}
			exeName := filepath.Base(exePath)
			fmt.Printf("%s already running.\n", exeName)
		} else {
			fmt.Printf("Can not open lockfile %s: %v\n", lockFile, err)
		}
		return
	}

	// 初始化shutdown context
	shutdown, cancel = context.WithCancel(context.Background())

	// 创建一个信号通道
	signalChan := make(chan os.Signal, 1)

	// 监听指定的信号
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// 在一个新的goroutine中等待信号
	go func() {
		select {
		case <-signalChan:
			runShutdownHook(f, lockFile)
		case <-shutdown.Done():
		}
	}()
	defer func() {
		f.Close()
		os.Remove(lockFile)
	}()

	// 获取并设置进程ID
	pid := os.Getpid()
	pidBytes := []byte(strconv.Itoa(pid))
	if err := ioutil.WriteFile(lockFile, pidBytes, 0644); err != nil {
		fmt.Printf("Can not write lockfile %s: %v\n", lockFile, err)
		return
	}

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
	for {
		ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
		if err == nil {
			ln.Close()
			break
		}
		port++
	}

	fmt.Printf("Listening on port %d\n", port)

	// 运行Web服务器
	r.Run(":" + strconv.Itoa(port))
}
