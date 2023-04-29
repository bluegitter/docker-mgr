package handlers

import (
	"bytes"
	"context"
	"embed"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type WebsocketWriter struct {
	conn *websocket.Conn
}

func (w *WebsocketWriter) Write(p []byte) (int, error) {
	err := w.conn.WriteMessage(websocket.TextMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func ConsoleHandler(c *gin.Context, templateFS embed.FS) {
	file, err := templateFS.Open("templates/console.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "Error reading console.html")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error reading console.html")
		return
	}

	reader := bytes.NewReader(content)
	http.ServeContent(c.Writer, c.Request, "console.html", time.Now(), reader)
}

func WsHandler(c *gin.Context, cli *client.Client) {
	containerID := c.Query("containerID")

	if containerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "containerID is required"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println(err)
		return
	}

	defer conn.Close()

	execConfig := types.ExecConfig{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          []string{"/bin/bash"},
	}

	exec, err := cli.ContainerExecCreate(context.Background(), containerID, execConfig)
	if err != nil {
		log.Fatal(err)
	}

	attach := types.ExecStartCheck{
		Tty: true,
	}

	resp, err := cli.ContainerExecAttach(context.Background(), exec.ID, attach)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Close()

	go func() {
		for {
			_, r, err := conn.NextReader()
			if err != nil {
				break
			}
			_, err = io.Copy(resp.Conn, r)
			if err != nil {
				break
			}
		}
	}()

	buf := make([]byte, 1024)
	for {
		n, err := resp.Reader.Read(buf)
		if err != nil {
			break
		}
		err = conn.WriteMessage(websocket.TextMessage, buf[:n])
		if err != nil {
			break
		}
	}
}
