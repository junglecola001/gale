package bridge

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gale/internal/logx"
	"gale/internal/model"
)

// startTest runs a bridge server with a fake engine behind it.
func startTest(t *testing.T, enabled bool) (*Server, *[]model.AddRequest) {
	t.Helper()
	var got []model.AddRequest
	s, err := Start(t.TempDir(), "9.9.9", func(req model.AddRequest) (int64, error) {
		got = append(got, req)
		return 42, nil
	}, func() bool { return enabled })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, &got
}

func infoOf(t *testing.T, s *Server) Info {
	t.Helper()
	info, err := ReadInfo(filepath.Dir(s.path))
	if err != nil {
		t.Fatalf("read info: %v", err)
	}
	return info
}

func post(t *testing.T, s *Server, token, body string) (*http.Response, httpReply) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(s.Port())+"/v1/add", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Gale-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	var out httpReply
	json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestAddRequiresTokenAndSwitch(t *testing.T) {
	s, got := startTest(t, true)
	info := infoOf(t, s)
	if info.Port != s.Port() {
		t.Fatalf("info = %+v", info)
	}

	if resp, _ := post(t, s, "", `{"url":"http://example.com/a.bin"}`); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token gave %d", resp.StatusCode)
	}
	if resp, _ := post(t, s, info.Token, `{"url":"http://example.com/a.bin"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("valid add gave %d", resp.StatusCode)
	}
	if len(*got) != 1 || (*got)[0].URL != "http://example.com/a.bin" {
		t.Fatalf("add not forwarded: %+v", *got)
	}
	if resp, _ := post(t, s, info.Token, `{"url":"ftp://example.com/a.bin"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("ftp url gave %d", resp.StatusCode)
	}

	off, _ := startTest(t, false)
	if resp, _ := post(t, off, infoOf(t, off).Token, `{"url":"http://example.com/a.bin"}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("capture off gave %d", resp.StatusCode)
	}
}

func TestPingMarksConnected(t *testing.T) {
	s, _ := startTest(t, true)
	info := infoOf(t, s)
	if s.Connected() {
		t.Fatalf("connected before a ping")
	}
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(s.Port())+"/v1/ping", nil)
	req.Header.Set("X-Gale-Token", info.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ping gave %d", resp.StatusCode)
	}
	if !s.Connected() || s.LastPing() == 0 {
		t.Fatalf("ping was not recorded")
	}
}

func TestHostProtocol(t *testing.T) {
	logs := t.TempDir()
	t.Cleanup(logx.Reset)
	s, got := startTest(t, true)
	in := &bytes.Buffer{}
	writeMessage(t, in, map[string]any{"type": "ping"})
	writeMessage(t, in, map[string]any{"type": "add", "url": "http://example.com/b.zip", "fileName": "b.zip"})
	out := &bytes.Buffer{}
	if code := RunHost(in, out, HostDirs{UserData: filepath.Dir(s.path), Logs: logs}); code != 0 {
		t.Fatalf("host exited with %d", code)
	}

	if first := readMessage(t, out); first["ok"] != true {
		t.Fatalf("ping reply = %v", first)
	}
	second := readMessage(t, out)
	if second["ok"] != true || second["id"] != float64(42) {
		t.Fatalf("add reply = %v", second)
	}
	if len(*got) != 1 || (*got)[0].FileName != "b.zip" {
		t.Fatalf("add not forwarded: %+v", *got)
	}
}

func TestHostReportsMissingApp(t *testing.T) {
	logs := t.TempDir()
	t.Cleanup(logx.Reset)
	// The app would be started from the test binary, which is not an app.
	restore := launchApp
	launchApp = func() {}
	t.Cleanup(func() { launchApp = restore })

	in := &bytes.Buffer{}
	writeMessage(t, in, map[string]any{"type": "add", "url": "http://example.com/c.bin"})
	writeMessage(t, in, map[string]any{"type": "ping"})
	out := &bytes.Buffer{}
	RunHost(in, out, HostDirs{UserData: t.TempDir(), Logs: logs})
	if first := readMessage(t, out); first["ok"] != false || first["error"] != "app_unavailable" {
		t.Fatalf("add reply without an app = %v", first)
	}
	if second := readMessage(t, out); second["error"] != "app_unavailable" {
		t.Fatalf("ping reply = %v", second)
	}
}

func writeMessage(t *testing.T, w io.Writer, v any) {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(body))); err != nil {
		t.Fatal(err)
	}
	w.Write(body)
}

func readMessage(t *testing.T, r *bytes.Buffer) map[string]any {
	t.Helper()
	body, err := readFrame(r)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}
