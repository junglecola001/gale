package bridge

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gale/internal/logx"
	"gale/internal/model"
)

// maxFrame is the largest native messaging frame Chrome sends.
const maxFrame = 1 << 20

// HostDirs are where the native host finds the running app and its log.
type HostDirs struct {
	UserData string // holds bridge.json
	Logs     string // holds native-host.log
}

// message is what the extension sends.
type message struct {
	Type      string `json:"type"`
	URL       string `json:"url"`
	Referrer  string `json:"referrer"`
	UserAgent string `json:"userAgent"`
	Cookies   string `json:"cookies"`
	FileName  string `json:"fileName"`
}

// reply is what the extension gets back.
type reply struct {
	OK      bool   `json:"ok"`
	ID      int64  `json:"id,omitempty"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// RunHost serves the native messaging connection on in and out until Chrome
// closes it. Nothing but frames may ever reach out: it is the pipe Chrome
// reads.
func RunHost(in io.Reader, out io.Writer, dirs HostDirs) int {
	if err := logx.Init(filepath.Join(dirs.Logs, "native-host.log")); err != nil {
		fmt.Fprint(out, "log unavailable")
	}
	defer logx.Logger().Info("native host exiting")

	for {
		body, err := readFrame(in)
		if err != nil {
			return 0
		}
		var msg message
		if err := json.Unmarshal(body, &msg); err != nil {
			writeFrame(out, reply{Error: "bad_message"})
			continue
		}
		logx.Logger().Info("native host message", "type", msg.Type)
		switch msg.Type {
		case "ping":
			writeFrame(out, pingReply(dirs))
		case "add":
			writeFrame(out, addReply(dirs, msg))
		default:
			writeFrame(out, reply{Error: "unknown_type"})
		}
	}
}

// pingReply only looks: the extension pings every 30 seconds, and starting
// the app for that would undo the user's decision to quit it.
func pingReply(dirs HostDirs) reply {
	info, err := ReadInfo(dirs.UserData)
	if err != nil {
		return reply{Error: "app_unavailable"}
	}
	resp, err := httpRequest(info, http.MethodGet, "/v1/ping", nil)
	if err != nil || !resp.OK {
		return reply{Error: "app_unavailable"}
	}
	return reply{OK: true, Version: resp.Version}
}

// addReply hands the download to the running app, starting it if need be.
func addReply(dirs HostDirs, msg message) reply {
	req := model.AddRequest{
		URL:       msg.URL,
		Referer:   msg.Referrer,
		UserAgent: msg.UserAgent,
		Cookies:   msg.Cookies,
		FileName:  msg.FileName,
	}
	deadline := time.Now().Add(5 * time.Second)
	launched := false
	for {
		info, err := ReadInfo(dirs.UserData)
		if err == nil {
			resp, err := httpRequest(info, http.MethodPost, "/v1/add", req)
			if err == nil && resp.OK {
				return reply{OK: true, ID: resp.ID, Version: resp.Version}
			}
			if err == nil && resp.Error != "" && !isUnavailable(resp.Error) {
				// The app answered and refused: retrying changes nothing.
				return reply{Error: resp.Error}
			}
		}
		if !launched {
			launchApp()
			launched = true
		}
		if time.Now().After(deadline) {
			return reply{Error: "app_unavailable"}
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// launchApp starts the app, here so that tests can keep it from starting
// them.
var launchApp = startApp

func isUnavailable(err string) bool {
	return err == "app_unavailable" || err == "connection refused"
}

// startApp launches the app without arguments, so it opens its window.
func startApp() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		logx.Logger().Warn("starting the app failed", "error", err)
		return
	}
	go cmd.Wait()
	logx.Logger().Info("started the app", "exe", exe)
}

type httpReply struct {
	OK      bool   `json:"ok"`
	ID      int64  `json:"id"`
	Version string `json:"version"`
	Error   string `json:"error"`
}

func httpRequest(info Info, method, path string, body any) (*httpReply, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", info.Port, path)
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Gale-Token", info.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out httpReply
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// readFrame reads one length-prefixed JSON message.
func readFrame(in io.Reader) ([]byte, error) {
	var size uint32
	if err := binary.Read(in, binary.LittleEndian, &size); err != nil {
		return nil, err
	}
	if size == 0 || size > maxFrame {
		return nil, fmt.Errorf("bridge: bad frame size %d", size)
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(in, body); err != nil {
		return nil, err
	}
	return body, nil
}

// writeFrame writes one length-prefixed JSON message.
func writeFrame(out io.Writer, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		return
	}
	if err := binary.Write(out, binary.LittleEndian, uint32(len(body))); err != nil {
		return
	}
	out.Write(body)
}
