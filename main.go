package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/bluegitter/docker-mgr/bootstrap"
	"github.com/bluegitter/docker-mgr/routes"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
)

//go:embed templates/*
var templateFS embed.FS
var portFlag *int

func main() {
	// 解析命令行参数
	portFlag = flag.Int("port", 8000, "The port to listen on")
	daemonFlag := flag.Bool("daemon", false, "run as a daemon process")
	flag.Parse()

	lockFile := bootstrap.GetLockFilePath()
	fmt.Printf("lockFile \033[32m%s\033[0m.\n", lockFile)

	ret := bootstrap.CheckAndRemoveLockFile(lockFile)
	if !ret {
		os.Exit(0)
	}

	if *daemonFlag {
		runDaemon()
		return
	} else {
		bootstrap.Init()
		run()
	}
}

func runDaemon() {
	// Fork the process to the background
	childPid := forkToBackground()

	// Print the child process PID and exit
	fmt.Printf("Forked to background, child process PID: %d\n", childPid)

	// Close standard file descriptors
	os.Stdin.Close()
	os.Stdout.Close()
	os.Stderr.Close()
}

func run() {

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

func forkToBackground() int {
	// Get the current executable path
	exePath, err := os.Executable()
	if err != nil {
		fmt.Println("无法获取可执行文件路径:", err)
		os.Exit(1)
	}

	argsWithoutDaemon := []string{}
	for _, arg := range os.Args[1:] {
		if arg != "--daemon" {
			argsWithoutDaemon = append(argsWithoutDaemon, arg)
		}
	}

	cmd := exec.Command(exePath, argsWithoutDaemon...)
	fmt.Printf("Running '%+v' in background.\n", cmd)

	// Set the attributes required for the child process
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	// Start the child process
	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to fork process: %v\n", err)
		os.Exit(1)
	}

	return cmd.Process.Pid
}
