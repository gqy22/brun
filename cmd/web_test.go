package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biotools/brun/internal"
)

// --- 测试辅助 ---

type testSrv struct {
	*WebServer
	mux    *http.ServeMux
	server *http.Server
	addr   string
}

func newTestServer(t *testing.T) (*testSrv, string) {
	t.Helper()
	store, err := internal.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("创建测试 store 失败: %v", err)
	}
	runDir := t.TempDir()
	runID := "test-run-001"
	run := &internal.Run{
		ID:     runID,
		Status: "running",
		RunDir: runDir,
	}
	if err := store.CreateRun(run); err != nil {
		t.Fatalf("创建测试 run 失败: %v", err)
	}

	os.WriteFile(filepath.Join(runDir, "stdout.o"), []byte("line1\nline2\n"), 0644)
	os.WriteFile(filepath.Join(runDir, "stderr.er"), []byte("err1\n"), 0644)

	ws := NewWebServer(store, "127.0.0.1", 0, os.DirFS("."), os.DirFS("."))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/runs/{id}", ws.apiGetRun)
	mux.HandleFunc("GET /api/runs/{id}/logs", ws.apiGetLogs)
	mux.HandleFunc("GET /api/runs/{id}/logs/stream", ws.apiStreamLogs)

	// 启动真实 HTTP server（用于 SSE 长连接测试）
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	ts := &testSrv{ws, mux, srv, ln.Addr().String()}
	t.Cleanup(func() { srv.Close() })
	return ts, runID
}

// doReq 通过 mux 路由请求，确保 PathValue 正确填充（用于普通 API 测试）
func (ts *testSrv) doReq(method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	w := httptest.NewRecorder()
	ts.mux.ServeHTTP(w, req)
	return w
}

func TestApiGetRun_ReturnsScriptSnapshotNameAndContent(t *testing.T) {
	srv, runID := newTestServer(t)
	runDir := srv.getTestRunDir(t, runID)
	script := "echo hello\n"
	if err := os.WriteFile(filepath.Join(runDir, "script.test.sh"), []byte(script), 0644); err != nil {
		t.Fatalf("写入脚本快照失败: %v", err)
	}

	w := srv.doReq("GET", "/api/runs/"+runID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["script_name"] != "test.sh" {
		t.Errorf("script_name = %q, want test.sh", resp["script_name"])
	}
	if resp["script"] != script {
		t.Errorf("script = %q, want %q", resp["script"], script)
	}
}

// fetchSSE 通过真实 HTTP 连接请求 SSE 端点，返回完整响应 body
func (ts *testSrv) fetchSSE(t *testing.T, urlPath string, timeout time.Duration) ([]byte, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+ts.addr+urlPath, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE 请求失败: %v", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode
}

// ===== 增量日志 API (offset) 测试 =====

func TestApiGetLogs_WithOffset_ReturnsOnlyIncrement(t *testing.T) {
	srv, runID := newTestServer(t)
	w := srv.doReq("GET", "/api/runs/"+runID+"/logs?stream=stdout&offset=6")

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	content := resp["content"].(string)

	if !strings.HasPrefix(content, "line2\n") {
		t.Errorf("content = %q, want prefix %q", content, "line2\\n")
	}
	if strings.Contains(content, "line1") {
		t.Error("增量内容不应包含 offset 之前的数据")
	}
}

func TestApiGetLogs_WithOffset_ReturnsCurrentSize(t *testing.T) {
	srv, runID := newTestServer(t)
	w := srv.doReq("GET", "/api/runs/"+runID+"/logs?stream=stdout&offset=0")

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	size := resp["size"].(float64)

	if size != 12 {
		t.Errorf("size = %v, want 12", size)
	}
}

func TestApiGetLogs_ZeroOffset_ReturnsFullContent(t *testing.T) {
	srv, runID := newTestServer(t)
	w := srv.doReq("GET", "/api/runs/"+runID+"/logs?stream=stdout&offset=0")

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	content := resp["content"].(string)

	if !strings.Contains(content, "line1") || !strings.Contains(content, "line2") {
		t.Errorf("全量内容应包含所有行, got %q", content)
	}
}

func TestApiGetLogs_OffsetExceedsFileSize_ReturnsEmptyContent(t *testing.T) {
	srv, runID := newTestServer(t)
	w := srv.doReq("GET", "/api/runs/"+runID+"/logs?stream=stdout&offset=9999")

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	content := resp["content"].(string)

	if content != "" {
		t.Errorf("超出文件大小的 offset 应返回空内容, got %q", content)
	}
}

func TestApiGetLogs_NonExistentRun_Returns404(t *testing.T) {
	srv, _ := newTestServer(t)
	w := srv.doReq("GET", "/api/runs/nonexistent/logs")

	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestApiGetLogs_WithTailAndOffset_TailAppliedToIncrement(t *testing.T) {
	srv, runID := newTestServer(t)
	w := srv.doReq("GET", "/api/runs/"+runID+"/logs?stream=stdout&offset=6&tail=1")

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	content := resp["content"].(string)

	if !strings.Contains(content, "line2") {
		t.Errorf("tail+offset 应返回 line2, got %q", content)
	}
}

// ===== SSE 日志流端点测试 =====

func TestSSEStreamLogs_ReturnsEventStreamContentType(t *testing.T) {
	srv, runID := newTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	req, _ := http.NewRequestWithContext(ctx, "GET",
		"http://"+srv.addr+"/api/runs/"+runID+"/logs/stream?stream=stdout", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}

	ct := resp.Header.Get("Content-Type")
	cancel()
	resp.Body.Close()
	if ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}

func TestSSEStreamLogs_SendsInitialContentAsSSE(t *testing.T) {
	srv, runID := newTestServer(t)
	// 立即标记为已完成，让 SSE 快速退出
	srv.store.UpdateRunStatus(runID, "success", 0, "", 100)
	body, _ := srv.fetchSSE(t, "/api/runs/"+runID+"/logs/stream?stream=stdout", 3*time.Second)

	if len(body) == 0 {
		t.Fatal("SSE 应该已发送初始数据")
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "data:") {
		t.Errorf("SSE 数据格式错误, 缺少 data: 前缀, body=%q", bodyStr[:minInt(len(bodyStr), 200)])
	}
	if !strings.Contains(bodyStr, "line1") {
		t.Errorf("SSE 数据应包含日志内容 line1, body=%q", bodyStr[:minInt(len(bodyStr), 200)])
	}
}

func TestSSEStreamLogs_PushesWhenFileGrows(t *testing.T) {
	srv, runID := newTestServer(t)
	runDir := srv.getTestRunDir(t, runID)
	logPath := filepath.Join(runDir, "stdout.o")

	done := make(chan []byte)
	go func() {
		data, _ := srv.fetchSSE(t, "/api/runs/"+runID+"/logs/stream?stream=stdout", 6*time.Second)
		done <- data
	}()

	time.Sleep(500 * time.Millisecond)
	os.WriteFile(logPath, []byte("line1\nline2\nnewline3\n"), 0644)
	time.Sleep(800 * time.Millisecond)
	srv.store.UpdateRunStatus(runID, "success", 0, "", 100)

	select {
	case body := <-done:
		bodyStr := string(body)
		if !strings.Contains(bodyStr, "newline3") {
			t.Errorf("SSE 应检测到文件增长并推送新内容, body=%q", truncateStr(bodyStr, 500))
		}
	case <-time.After(8 * time.Second):
		t.Fatal("SSE 超时等待文件增长事件")
	}
}

func TestSSEStreamLogs_NonExistentRun_Returns404(t *testing.T) {
	srv, _ := newTestServer(t)
	body, code := srv.fetchSSE(t, "/api/runs/nonexistent/logs/stream", 2*time.Second)

	if code != 404 {
		t.Errorf("status = %d, want 404", code)
	}
	if strings.Contains(string(body), "event-stream") {
		t.Error("不存在的 run 不应返回 SSE 流")
	}
}

func TestSSEStreamLogs_FinishedRun_SendsCompleteAndCloses(t *testing.T) {
	srv, runID := newTestServer(t)
	srv.store.UpdateRunStatus(runID, "success", 0, "", 100)

	body, code := srv.fetchSSE(t, "/api/runs/"+runID+"/logs/stream?stream=stdout", 3*time.Second)
	if code != 200 {
		t.Fatalf("status = %d, want 200", code)
	}
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "line1") {
		t.Errorf("完成的 run 应先发送完整日志, body=%q", truncateStr(bodyStr, 200))
	}
	if !strings.Contains(bodyStr, `"done":true`) {
		t.Errorf("完成的 run 应发送 done 事件, body=%q", truncateStr(bodyStr, 200))
	}
}

// ===== 辅助函数 =====

func (s *WebServer) getTestRunDir(t *testing.T, runID string) string {
	t.Helper()
	run, err := s.store.GetRun(runID)
	if err != nil {
		t.Fatalf("获取 run 失败: %v", err)
	}
	return run.RunDir
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// scanSSEEvents 从 SSE body 中解析出所有事件
func scanSSEEvents(body string) []map[string]any {
	var events []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(body))
	var currentData strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			currentData.WriteString(strings.TrimPrefix(line, "data: "))
		} else if line == "" && currentData.Len() > 0 {
			var evt map[string]any
			json.Unmarshal([]byte(currentData.String()), &evt)
			events = append(events, evt)
			currentData.Reset()
		}
	}
	return events
}
