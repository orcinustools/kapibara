package api

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/orcinustools/kapibara/pkg/kube"
)

// execUpgrader upgrades the exec endpoint to a WebSocket. The request is already
// authenticated (?token=…), so we allow any origin.
var execUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// execClientMsg is a message from the browser terminal: either stdin `data` or a
// `resize` event.
type execClientMsg struct {
	Data   string `json:"data,omitempty"`
	Resize *struct {
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	} `json:"resize,omitempty"`
}

// handleExec opens an interactive shell into a pod over a WebSocket. Query
// params: service (pick first pod), pod (explicit), container, shell.
// Client → server: JSON {data} (stdin) or {resize:{cols,rows}}.
// Server → client: raw terminal output (binary frames).
func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	if s.Kube == nil {
		writeError(w, http.StatusServiceUnavailable, "cluster access unavailable (no kubeconfig)")
		return
	}

	pod := r.URL.Query().Get("pod")
	service := r.URL.Query().Get("service")
	container := r.URL.Query().Get("container")
	if pod == "" {
		for _, u := range s.projectUnits(p.ID) {
			pods, err := s.Orcinus.Pods(r.Context(), u.OrcinusProject)
			if err != nil {
				continue
			}
			if pod = pickPod(pods, service); pod != "" {
				break
			}
		}
		if pod == "" {
			writeError(w, http.StatusNotFound, "no pods found for project/service")
			return
		}
	}

	// The command: honor an explicit ?shell=, else prefer bash and fall back to
	// sh so it works on both debian- and alpine-based images.
	var command []string
	if sh := r.URL.Query().Get("shell"); sh != "" {
		command = []string{sh}
	} else {
		command = []string{"/bin/sh", "-c", "export TERM=xterm-256color; exec /bin/bash 2>/dev/null || exec /bin/sh"}
	}

	conn, err := execUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the error
	}
	defer conn.Close()

	stdinR, stdinW := io.Pipe()
	resize := make(chan kube.TerminalSize, 4)
	var wmu sync.Mutex
	writeWS := func(mt int, b []byte) error {
		wmu.Lock()
		defer wmu.Unlock()
		return conn.WriteMessage(mt, b)
	}

	// Reader: browser → stdin / resize.
	go func() {
		defer stdinW.Close()
		defer close(resize)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg execClientMsg
			if json.Unmarshal(data, &msg) == nil && (msg.Resize != nil || msg.Data != "") {
				if msg.Resize != nil {
					select {
					case resize <- kube.TerminalSize{Width: msg.Resize.Cols, Height: msg.Resize.Rows}:
					default:
					}
					continue
				}
				_, _ = io.WriteString(stdinW, msg.Data)
				continue
			}
			// Not a control message → treat as raw stdin.
			_, _ = stdinW.Write(data)
		}
	}()

	// Terminal output → browser.
	out := writerFunc(func(b []byte) (int, error) {
		if err := writeWS(websocket.BinaryMessage, b); err != nil {
			return 0, err
		}
		return len(b), nil
	})

	err = s.Kube.ExecTTY(r.Context(), s.Cfg.Namespace, pod, container, command, stdinR, out, resize)
	msg := "\r\n\x1b[90m[session ended]\x1b[0m\r\n"
	if err != nil {
		msg = "\r\n\x1b[31m[session ended: " + err.Error() + "]\x1b[0m\r\n"
	}
	_ = writeWS(websocket.BinaryMessage, []byte(msg))
	_ = writeWS(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}

// writerFunc adapts a function to io.Writer.
type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }
