package bootstrap

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

func Init() {
	lockFile := getLockFilePath()
	fmt.Printf("lockFile \033[32m%s\033[0m.\n", lockFile)

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
			pid := getExistPidFromLockFile(lockFile)
			fmt.Printf("Program '%s' already running, PID=%s.\n", exeName, pid)
		} else {
			fmt.Printf("Can not open lockfile %s: %v\n", lockFile, err)
		}
		os.Exit(0)
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

	// 获取并设置进程ID
	pid := os.Getpid()
	pidBytes := []byte(strconv.Itoa(pid))
	if err := ioutil.WriteFile(lockFile, pidBytes, 0644); err != nil {
		fmt.Printf("Can not write lockfile %s: %v\n", lockFile, err)
		return
	}
}

func getExistPidFromLockFile(lockFile string) string {
	// Read PID from the lockfile
	existingLockFile, err := os.Open(lockFile)
	if err != nil {
		fmt.Printf("Can not open existing lockfile %s: %v\n", lockFile, err)
		os.Exit(0)
	}
	defer existingLockFile.Close()

	pidBuf := make([]byte, 16)
	n, err := existingLockFile.Read(pidBuf)
	if err != nil {
		fmt.Printf("Can not read from lockfile %s: %v\n", lockFile, err)
		os.Exit(0)
	}

	existingPid := strings.TrimSpace(string(pidBuf[:n]))
	return existingPid
}

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

func EnsurePortNotUsed(portFlag *int) int {
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
			os.Exit(0)
		}
		port++
	}

	pid := os.Getpid()
	fmt.Printf("Listening on port \033[32m%d\033[0m, PID=\033[32m%d\033[0m.\n", port, pid)
	return port
}
