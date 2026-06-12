package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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
	store, err := internal.NewStore(filepath.Join(fastTempDir(t), "test.db"))
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

	ws := NewWebServer(store, "127.0.0.1", 0, os.DirFS("../web/templates"), os.DirFS("../web/static"))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/runs", ws.apiListRuns)
	mux.HandleFunc("GET /api/runs/{id}", ws.apiGetRun)
	mux.HandleFunc("GET /api/runs/{id}/logs", ws.apiGetLogs)
	mux.HandleFunc("GET /api/runs/{id}/logs/stream", ws.apiStreamLogs)
	mux.HandleFunc("GET /api/runs/{id}/processes", ws.apiGetProcesses)
	mux.HandleFunc("GET /api/hosts", ws.apiHosts)
	mux.HandleFunc("GET /api/users", ws.apiUsers)

	// 启动真实 HTTP server（用于 SSE 长连接测试）
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	ts := &testSrv{ws, mux, srv, ln.Addr().String()}
	t.Cleanup(func() { srv.Close() })
	return ts, runID
}

func fastTempDir(t *testing.T) string {
	t.Helper()
	base := "/dev/shm"
	if info, err := os.Stat(base); err != nil || !info.IsDir() {
		base = ""
	}
	dir, err := os.MkdirTemp(base, "brun-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
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

func TestApiGetRun_ReturnsDiagnostics(t *testing.T) {
	srv, runID := newTestServer(t)
	runDir := srv.getTestRunDir(t, runID)
	writer := internal.NewDiagnosticWriter(runDir)
	writer.Warning("metadata_write_failed", "metadata.yaml 写入失败", "permission denied")

	w := srv.doReq("GET", "/api/runs/"+runID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Diagnostics []internal.DiagnosticEvent `json:"diagnostics"`
		Summary     internal.DiagnosticSummary `json:"diagnostic_summary"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Summary.WarningCount != 1 {
		t.Fatalf("WarningCount = %d, want 1", resp.Summary.WarningCount)
	}
	if len(resp.Diagnostics) != 1 || resp.Diagnostics[0].Code != "metadata_write_failed" {
		t.Fatalf("diagnostics = %+v, want metadata_write_failed", resp.Diagnostics)
	}
}

func TestApiGetRun_ReturnsCondaMetadata(t *testing.T) {
	srv, runID := newTestServer(t)
	run, err := srv.store.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.store.DeleteRun(runID); err != nil {
		t.Fatal(err)
	}
	run.CondaStatus = "ok"
	run.CondaEnv = "rnaseq"
	run.CondaPrefix = "/opt/conda/envs/rnaseq"
	run.PythonVersion = "Python 3.11.8"
	if err := srv.store.CreateRun(run); err != nil {
		t.Fatal(err)
	}

	w := srv.doReq("GET", "/api/runs/"+runID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["conda_status"] != "ok" || resp["conda_env"] != "rnaseq" || resp["python_version"] != "Python 3.11.8" {
		t.Fatalf("unexpected conda metadata: %+v", resp)
	}
}

func TestApiGetRun_ReturnsResourceMetadata(t *testing.T) {
	srv, runID := newTestServer(t)
	run, err := srv.store.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.store.DeleteRun(runID); err != nil {
		t.Fatal(err)
	}
	run.ResourceSupported = true
	run.ResourceStatus = "ok"
	run.PeakRSSKB = 1024
	run.CPUTimeMs = 25
	if err := srv.store.CreateRun(run); err != nil {
		t.Fatal(err)
	}

	w := srv.doReq("GET", "/api/runs/"+runID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["resource_supported"] != true || resp["resource_status"] != "ok" {
		t.Fatalf("unexpected resource metadata: %+v", resp)
	}
	if resp["peak_rss_kb"] != float64(1024) || resp["cpu_time_ms"] != float64(25) {
		t.Fatalf("unexpected resource values: %+v", resp)
	}
}

func TestApiGetRun_ReturnsHostnameUsernameStatus(t *testing.T) {
	srv, runID := newTestServer(t)
	run, err := srv.store.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.store.DeleteRun(runID); err != nil {
		t.Fatal(err)
	}
	run.Hostname = "devbox"
	run.HostnameStatus = "ok"
	run.Username = "user"
	run.UsernameStatus = "ok"
	if err := srv.store.CreateRun(run); err != nil {
		t.Fatal(err)
	}

	w := srv.doReq("GET", "/api/runs/"+runID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["hostname"] != "devbox" || resp["hostname_status"] != "ok" {
		t.Fatalf("unexpected hostname metadata: %+v", resp)
	}
	if resp["username"] != "user" || resp["username_status"] != "ok" {
		t.Fatalf("unexpected username metadata: %+v", resp)
	}
}

func TestApiListRuns_DisplayStatusFilter(t *testing.T) {
	srv, runID := newTestServer(t)
	run, _ := srv.store.GetRun(runID)

	// 加两个 run：一个 success 无 warning，一个 success 有 warning
	if err := srv.store.UpdateRunStatus(runID, "success", 0, "2026-06-11T00:00:00Z", 100); err != nil {
		t.Fatal(err)
	}
	if err := srv.store.UpdateRunDiagnostics(runID, internal.DiagnosticSummary{WarningCount: 2, LastCode: "x", LastAt: "2026-06-11T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	clean := &internal.Run{ID: "clean-1", Status: "success", StartedAt: "2026-06-11T00:00:00Z", RunDir: "/t", CWD: "/t"}
	if err := srv.store.CreateRun(clean); err != nil {
		t.Fatal(err)
	}

	// 1. 不带过滤：两个都返回
	w := srv.doReq("GET", "/api/runs")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var rows []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(rows))
	}

	// 2. display_status=success_with_warnings：只命中带 warning 的那个
	w2 := srv.doReq("GET", "/api/runs?display_status=success_with_warnings")
	if w2.Code != 200 {
		t.Fatalf("status = %d, want 200", w2.Code)
	}
	var rows2 []map[string]any
	if err := json.NewDecoder(w2.Body).Decode(&rows2); err != nil {
		t.Fatal(err)
	}
	if len(rows2) != 1 || rows2[0]["id"] != runID {
		t.Fatalf("expected only warned run, got %+v", rows2)
	}
	if rows2[0]["display_status"] != "success_with_warnings" {
		t.Errorf("display_status = %v, want success_with_warnings", rows2[0]["display_status"])
	}
	if rows2[0]["status"] != "success" {
		t.Errorf("status = %v, want success", rows2[0]["status"])
	}

	_ = run
}

func TestApiListRuns_DisplayStatusFilter_FailedAndCancelled(t *testing.T) {
	srv, _ := newTestServer(t)

	// 准备三种状态的 run，每种都带一个 warning，让 DisplayStatus 切到 _with_warnings 变体
	failed := &internal.Run{ID: "fw1", Status: "failed", StartedAt: "2026-06-11T00:00:00Z", RunDir: "/t", CWD: "/t", DiagWarningCount: 1}
	if err := srv.store.CreateRun(failed); err != nil {
		t.Fatal(err)
	}
	cancelled := &internal.Run{ID: "cw1", Status: "cancelled", StartedAt: "2026-06-11T00:00:01Z", RunDir: "/t", CWD: "/t", DiagWarningCount: 2}
	if err := srv.store.CreateRun(cancelled); err != nil {
		t.Fatal(err)
	}
	plain := &internal.Run{ID: "plain1", Status: "failed", StartedAt: "2026-06-11T00:00:02Z", RunDir: "/t", CWD: "/t"}
	if err := srv.store.CreateRun(plain); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		query  string
		wantID string
		wantDS string
	}{
		{"failed_with_warnings", "?display_status=failed_with_warnings", "fw1", "failed_with_warnings"},
		{"cancelled_with_warnings", "?display_status=cancelled_with_warnings", "cw1", "cancelled_with_warnings"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := srv.doReq("GET", "/api/runs"+tc.query)
			if w.Code != 200 {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			var rows []map[string]any
			if err := json.NewDecoder(w.Body).Decode(&rows); err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 {
				t.Fatalf("expected 1 row, got %d (%+v)", len(rows), rows)
			}
			if rows[0]["id"] != tc.wantID {
				t.Errorf("id = %v, want %v", rows[0]["id"], tc.wantID)
			}
			if rows[0]["display_status"] != tc.wantDS {
				t.Errorf("display_status = %v, want %v", rows[0]["display_status"], tc.wantDS)
			}
		})
	}
}

func TestApiListRuns_HostAndUserFilter(t *testing.T) {
	srv, _ := newTestServer(t)

	// 覆盖默认 run + 加三个
	for _, id := range []string{"a", "b", "c"} {
		run := &internal.Run{ID: id, Status: "success", StartedAt: "2026-06-11T00:00:00Z", RunDir: "/t", CWD: "/t"}
		if id == "a" {
			run.Hostname = "devbox"
			run.Username = "alice"
		} else if id == "b" {
			run.Hostname = "devbox"
			run.Username = "bob"
		} else {
			run.Hostname = "build"
			run.Username = "alice"
		}
		if err := srv.store.CreateRun(run); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name    string
		query   string
		wantIDs []string
	}{
		{"no filter", "", []string{"a", "b", "c", "test-run-001"}},
		{"host=devbox", "?host=devbox", []string{"a", "b"}},
		{"user=alice", "?user=alice", []string{"a", "c"}},
		{"host=devbox&user=alice", "?host=devbox&user=alice", []string{"a"}},
		{"host=unknown", "?host=ghost", []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := srv.doReq("GET", "/api/runs"+tc.query)
			if w.Code != 200 {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			var rows []map[string]any
			if err := json.NewDecoder(w.Body).Decode(&rows); err != nil {
				t.Fatal(err)
			}
			gotIDs := make([]string, 0, len(rows))
			for _, r := range rows {
				if id, ok := r["id"].(string); ok {
					gotIDs = append(gotIDs, id)
				}
			}
			sort.Strings(gotIDs)
			wantSorted := tc.wantIDs
			if wantSorted == nil {
				wantSorted = []string{}
			} else {
				wantSorted = append([]string{}, tc.wantIDs...)
				sort.Strings(wantSorted)
			}
			if !reflect.DeepEqual(gotIDs, wantSorted) {
				t.Errorf("ids = %v, want %v", gotIDs, tc.wantIDs)
			}
		})
	}
}

func TestApiHostsAndUsers(t *testing.T) {
	srv, _ := newTestServer(t)
	run := &internal.Run{ID: "ha", Status: "success", Hostname: "devbox", Username: "alice", StartedAt: "2026-06-11T00:00:00Z", RunDir: "/t", CWD: "/t"}
	if err := srv.store.CreateRun(run); err != nil {
		t.Fatal(err)
	}
	run2 := &internal.Run{ID: "hb", Status: "success", Hostname: "build", Username: "bob", StartedAt: "2026-06-11T00:00:01Z", RunDir: "/t", CWD: "/t"}
	if err := srv.store.CreateRun(run2); err != nil {
		t.Fatal(err)
	}

	w := srv.doReq("GET", "/api/hosts")
	var hosts []string
	if err := json.NewDecoder(w.Body).Decode(&hosts); err != nil {
		t.Fatal(err)
	}
	wantHosts := map[string]bool{"devbox": true, "build": true}
	if len(hosts) != 2 {
		t.Errorf("hosts = %v, want 2 entries", hosts)
	}
	for _, h := range hosts {
		if !wantHosts[h] {
			t.Errorf("unexpected host %q", h)
		}
	}

	w2 := srv.doReq("GET", "/api/users")
	var users []string
	if err := json.NewDecoder(w2.Body).Decode(&users); err != nil {
		t.Fatal(err)
	}
	wantUsers := map[string]bool{"alice": true, "bob": true}
	if len(users) != 2 {
		t.Errorf("users = %v, want 2 entries", users)
	}
	for _, u := range users {
		if !wantUsers[u] {
			t.Errorf("unexpected user %q", u)
		}
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
	status := resp["status"].(string)

	if size != 12 {
		t.Errorf("size = %v, want 12", size)
	}
	if status != "ok" {
		t.Errorf("status = %q, want ok", status)
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

func TestApiGetLogs_MissingLogReportsStatus(t *testing.T) {
	srv, runID := newTestServer(t)
	runDir := srv.getTestRunDir(t, runID)
	if err := os.Remove(filepath.Join(runDir, "stdout.o")); err != nil {
		t.Fatalf("remove stdout: %v", err)
	}

	w := srv.doReq("GET", "/api/runs/"+runID+"/logs?stream=stdout")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "missing" {
		t.Fatalf("log status = %q, want missing", resp["status"])
	}
	if resp["content"] != "" {
		t.Fatalf("content = %q, want empty", resp["content"])
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

func TestReadLogFromOffset(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "stdout.o")
	if err := os.WriteFile(path, []byte("line1\nline2\n"), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	size, data, err := readLogFromOffset(path, 6)
	if err != nil {
		t.Fatalf("readLogFromOffset() error = %v", err)
	}
	if size != 12 {
		t.Fatalf("size = %d, want 12", size)
	}
	if got := string(data); got != "line2\n" {
		t.Fatalf("data = %q, want %q", got, "line2\n")
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

func TestHealthCheckMarksZombieRunWithEndedAtAndDuration(t *testing.T) {
	srv, runID := newTestServer(t)
	runDir := srv.getTestRunDir(t, runID)
	if err := os.WriteFile(filepath.Join(runDir, ".pid"), []byte("999999\n"), 0644); err != nil {
		t.Fatalf("写入 pid 失败: %v", err)
	}

	srv.checkRunningTasks()

	run, err := srv.store.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run.Status != "failed" {
		t.Fatalf("Status = %q, want failed", run.Status)
	}
	if run.EndedAt == "" {
		t.Fatal("EndedAt should be set for zombie run")
	}
	if run.DurationMs < 0 {
		t.Fatalf("DurationMs = %d, want >= 0", run.DurationMs)
	}
}

func TestApiGetProcesses_ReturnsSummary(t *testing.T) {
	srv, runID := newTestServer(t)
	runDir := srv.getTestRunDir(t, runID)
	if err := os.WriteFile(filepath.Join(runDir, ".pid"), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644); err != nil {
		t.Fatalf("写入 pid 失败: %v", err)
	}

	w := srv.doReq("GET", "/api/runs/"+runID+"/processes")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Processes []ProcessInfo     `json:"processes"`
		Summary   RunProcessSummary `json:"summary"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Summary.RootPID != os.Getpid() {
		t.Fatalf("root_pid = %d, want %d", resp.Summary.RootPID, os.Getpid())
	}
	if resp.Summary.LastLogUpdate == "" {
		t.Fatal("last_log_update should not be empty")
	}
	if resp.Summary.LastLogStatus != "ok" {
		t.Fatalf("last_log_status = %q, want ok", resp.Summary.LastLogStatus)
	}
	if resp.Summary.ProcessCount < 1 {
		t.Fatalf("process_count = %d, want >= 1", resp.Summary.ProcessCount)
	}
}

func TestReadLastLogUpdateReportsMissing(t *testing.T) {
	updatedAt, ago, status := readLastLogUpdate(t.TempDir())
	if updatedAt != "" || ago != "" || status != "missing" {
		t.Fatalf("readLastLogUpdate() = %q, %q, %q; want empty, empty, missing", updatedAt, ago, status)
	}
}

func TestWebServerExplicitPortDoesNotAutoIncrement(t *testing.T) {
	store, err := internal.NewStore(filepath.Join(fastTempDir(t), "test.db"))
	if err != nil {
		t.Fatalf("创建测试 store 失败: %v", err)
	}
	defer store.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	ws := NewWebServer(store, "127.0.0.1", port, os.DirFS("../web/templates"), os.DirFS("../web/static"))
	ws.SetAutoIncrementPort(false)
	err = ws.ListenAndServe()
	if err == nil {
		t.Fatal("ListenAndServe() error = nil, want port unavailable")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("端口 %d 不可用", port)) {
		t.Fatalf("error = %q, want explicit port unavailable", err)
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
