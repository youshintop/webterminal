package terminal

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/gorilla/websocket"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const END_OF_TRANSMISSION = "\u0004"

type PtyHandler interface {
	io.Reader
	io.Writer
	remotecommand.TerminalSizeQueue
}

// TerminalSession implements PtyHandler (using a SockJS connection)
type TerminalSession struct {
	ws       *websocket.Conn
	sizeChan chan remotecommand.TerminalSize
	doneChan chan struct{}
}

type TerminalMessage struct {
	Op   string `json:"op"`
	Data string `json:"data,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
}

// TerminalSize handles pty->process resize events
// Called in a loop from remotecommand as long as the process is running
func (t TerminalSession) Next() *remotecommand.TerminalSize {
	select {
	case size := <-t.sizeChan:
		return &size
	case <-t.doneChan:
		return nil
	}
}

// Read handles pty->process messages (stdin, resize)
// Called in a loop from remotecommand as long as the process is running
func (t TerminalSession) Read(p []byte) (int, error) {
	_, m, err := t.ws.ReadMessage()
	if err != nil {
		// Send terminated signal to process to avoid resource leak
		return copy(p, END_OF_TRANSMISSION), err
	}
	var msg TerminalMessage
	if err = json.Unmarshal(m, &msg); err != nil {
		return copy(p, END_OF_TRANSMISSION), err
	}
	switch msg.Op {
	case "stdin":
		return copy(p, msg.Data), nil
	case "resize":
		//t.sizeChan <- remotecommand.TerminalSize{Width: msg.Cols, Height: msg.Rows}
		return 0, nil
	default:
		return copy(p, END_OF_TRANSMISSION), fmt.Errorf("unknown message type '%s'", msg.Op)
	}
}

// Write handles process->pty stdout
// Called from remotecommand whenever there is any output
func (t TerminalSession) Write(p []byte) (int, error) {
	msg, err := json.Marshal(TerminalMessage{
		Op:   "stdout",
		Data: string(p),
	})
	if err != nil {
		return 0, err
	}
	if err := t.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Toast can be used to send the user any OOB messages
// hterm puts these in the center of the terminal
func (t TerminalSession) Toast(p string) error {
	if err := t.ws.WriteMessage(websocket.TextMessage, []byte(p)); err != nil {
		return err
	}
	return nil
}

/**
 * 向指定容器执行命令
 */
func startProcess(k8sClient kubernetes.Interface, cfg *rest.Config, cmd []string, ptyHandler PtyHandler) error {
	namespace := "default"
	podName := "nginx"
	containerName := "nginx"
	req := k8sClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")

	req.VersionedParams(&v1.PodExecOptions{
		Container: containerName,
		Command:   cmd,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return err
	}

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:             ptyHandler,
		Stdout:            ptyHandler,
		Stderr:            ptyHandler,
		TerminalSizeQueue: ptyHandler,
		Tty:               true,
	})
	if err != nil {
		return err
	}

	return nil
}

func WaitForTerminal(ws *websocket.Conn, client kubernetes.Interface, config *rest.Config) {
	pyHandler := TerminalSession{}
	pyHandler.ws = ws

	err := startProcess(client, config, []string{"sh"}, pyHandler)
	if err != nil {
		panic(err)
	}
}
