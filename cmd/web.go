package cmd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/biotools/brun/internal"
)

type WebServer struct {
	store   *internal.Store
	addr    string
	port    int
	tmplDir fs.FS
	static  fs.FS
}

func NewWebServer(store *internal.Store, addr string, port int, tmplFS, staticFS fs.FS) *WebServer {
	tmplSub, _ := fs.Sub(tmplFS, "web/templates")
	staticSub, _ := fs.Sub(staticFS, "web/static")
	return &WebServer{
		store:   store,
		addr:    addr,
		port:    port,
		tmplDir: tmplSub,
		static:  staticSub,
	}
}

func (s *WebServer) ListenAndServe() error {
	mux := http.NewServeMux()

	// Static files (must register before catch-all routes)
	fileServer := http.FileServer(http.FS(s.static))
	mux.Handle("GET /static/", http.StripPrefix("/static/", fileServer))

	// API routes
	mux.HandleFunc("GET /api/runs", s.apiListRuns)
	mux.HandleFunc("GET /api/runs/{id}", s.apiGetRun)
	mux.HandleFunc("GET /api/runs/{id}/logs", s.apiGetLogs)
	mux.HandleFunc("GET /api/runs/{id}/logs/stream", s.apiStreamLogs)
	mux.HandleFunc("GET /api/runs/{id}/artifacts", s.apiGetArtifacts)
	mux.HandleFunc("POST /api/runs/{id}/rerun", s.apiRerun)
	mux.HandleFunc("POST /api/runs/{id}/kill", s.apiKill)
	mux.HandleFunc("DELETE /api/runs/{id}", s.apiDeleteRun)
	mux.HandleFunc("GET /api/projects", s.apiProjects)
	mux.HandleFunc("GET /api/tags", s.apiTags)

	// Page routes (catch-all last)
	mux.HandleFunc("GET /", s.pageIndex)
	mux.HandleFunc("GET /run/{id}", s.pageRun)

	for attempt := 0; ; attempt++ {
		p := s.port + attempt
		addr := fmt.Sprintf("%s:%d", s.addr, p)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			if attempt < 20 {
				continue
			}
			return fmt.Errorf("端口 %d-%d 均被占用，请手动指定 --port", s.port, p-1)
		}
		if attempt > 0 {
			internal.Log().Warn("web_port_in_use", "port", s.port, "using", p)
		}
		internal.Log().Info("web_started", "addr", addr, "port", p)
		s.printLANAddrs(p)

		go s.healthCheckLoop(60 * time.Second)

		srv := &http.Server{Handler: mux}
		return srv.Serve(ln)
	}
}

// --- JSON API handlers ---

func (s *WebServer) apiListRuns(w http.ResponseWriter, r *http.Request) {
	internal.Log().Info("api_request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
	project := r.URL.Query().Get("project")
	status := r.URL.Query().Get("status")
	tag := r.URL.Query().Get("tag")
	search := r.URL.Query().Get("search")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	runs, err := s.store.ListRuns(limit, project, status, tag, search, "", "")
	if err != nil {
		httpError(w, err.Error(), 500)
		return
	}

	type runRow struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Project   string `json:"project"`
		Status    string `json:"status"`
		Duration  string `json:"duration"`
		Command   string `json:"command"`
		StartedAt string `json:"started_at"`
	}

	rows := make([]runRow, len(runs))
	for i, run := range runs {
		rows[i] = runRow{
			ID:        run.ID,
			Name:      run.Name,
			Project:   run.Project,
			Status:    run.Status,
			Duration:  DurationString(run.DurationMs),
			Command:   truncate(run.Command, 80),
			StartedAt: run.StartedAt,
		}
	}
	jsonResponse(w, rows)
}

func (s *WebServer) apiGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.store.GetRun(id)
	if err != nil {
		httpError(w, err.Error(), 404)
		return
	}

	tags, _ := s.store.GetTags(run.ID)
	note, _ := s.store.GetNote(run.ID)

	jsonResponse(w, map[string]any{
		"id":          run.ID,
		"name":        run.Name,
		"project":     run.Project,
		"cwd":         run.CWD,
		"command":     run.Command,
		"script":      readInputScript(run.RunDir),
		"status":      run.Status,
		"exit_code":   run.ExitCode,
		"started_at":  run.StartedAt,
		"ended_at":    run.EndedAt,
		"duration_ms": run.DurationMs,
		"duration":    DurationString(run.DurationMs),
		"hostname":    run.Hostname,
		"username":    run.Username,
		"git_repo":    run.GitRepo,
		"git_branch":  run.GitBranch,
		"git_commit":  run.GitCommit,
		"git_dirty":   run.GitDirty,
		"peak_rss_kb": run.PeakRSSKB,
		"cpu_time_ms": run.CPUTimeMs,
		"run_dir":     run.RunDir,
		"tags":        tags,
		"note":        note,
	})
}

func (s *WebServer) apiGetLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.store.GetRun(id)
	if err != nil {
		httpError(w, err.Error(), 404)
		return
	}

	stream := r.URL.Query().Get("stream")
	if stream == "" {
		stream = "stdout"
	}
	tailN, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	var logPath string
	switch stream {
	case "stderr":
		logPath = filepath.Join(run.RunDir, "stderr.er")
	default:
		logPath = filepath.Join(run.RunDir, "stdout.o")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		jsonResponse(w, map[string]string{"content": "", "stream": stream})
		return
	}

	fileSize := len(data)

	if offset > 0 {
		if offset >= fileSize {
			jsonResponse(w, map[string]any{
				"content": "",
				"stream":  stream,
				"size":    fileSize,
			})
			return
		}
		data = data[offset:]
	}

	content := string(data)
	if tailN > 0 {
		content = TailLog(content, tailN)
	}

	jsonResponse(w, map[string]any{
		"content": content,
		"stream":  stream,
		"size":    fileSize,
	})
}

// apiStreamLogs SSE 日志流端点 — 实时推送日志增量
func (s *WebServer) apiStreamLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.store.GetRun(id)
	if err != nil {
		httpError(w, err.Error(), 404)
		return
	}

	stream := r.URL.Query().Get("stream")
	if stream == "" {
		stream = "stdout"
	}

	var logPath string
	switch stream {
	case "stderr":
		logPath = filepath.Join(run.RunDir, "stderr.er")
	default:
		logPath = filepath.Join(run.RunDir, "stdout.o")
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, "streaming not supported", 500)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sendSSE := func(data map[string]any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	data, readErr := os.ReadFile(logPath)
	if readErr != nil {
		data = []byte{}
	}
	prevSize := len(data)

	sendSSE(map[string]any{
		"content": string(data),
		"size":    prevSize,
	})

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			run, checkErr := s.store.GetRun(id)
			if checkErr != nil {
				return
			}
			if run.Status != "running" {
				sendSSE(map[string]any{"done": true})
				return
			}

			data, readErr = os.ReadFile(logPath)
			if readErr != nil {
				continue
			}
			if len(data) > prevSize {
				sendSSE(map[string]any{
					"content": string(data[prevSize:]),
					"size":    len(data),
				})
				prevSize = len(data)
			}
		}
	}
}

func (s *WebServer) apiGetArtifacts(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	arts, err := s.store.GetArtifacts(id)
	if err != nil {
		httpError(w, err.Error(), 404)
		return
	}

	type artRow struct {
		Kind   string `json:"kind"`
		Status string `json:"status"`
		Size   string `json:"size"`
		Path   string `json:"path"`
	}

	rows := make([]artRow, len(arts))
	for i, a := range arts {
		rows[i] = artRow{
			Kind:   a.Kind,
			Status: a.Status,
			Size:   FormatSize(a.Size),
			Path:   a.Path,
		}
	}
	jsonResponse(w, rows)
}

func (s *WebServer) apiRerun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.store.GetRun(id)
	if err != nil {
		httpError(w, err.Error(), 404)
		return
	}

	cmdParts := strings.Fields(run.Command)
	if len(cmdParts) == 0 {
		httpError(w, "no command to rerun", 400)
		return
	}

	// 生成全新任务记录
	newID := internal.GenerateRunID()
	newDir := internal.RunDir(newID)
	os.MkdirAll(newDir, 0755)

	stdoutPath := filepath.Join(newDir, "stdout.o")
	stderrPath := filepath.Join(newDir, "stderr.er")

	newRecord := &internal.Run{
		ID:        newID,
		Name:      run.Name,
		Project:   run.Project,
		CWD:       run.CWD,
		Command:   run.Command,
		Status:    "running",
		RunDir:    newDir,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.store.CreateRun(newRecord); err != nil {
		httpError(w, "创建任务失败: "+err.Error(), 500)
		return
	}
	SaveCommandFile(newDir, run.Command)
	SaveEnvFile(newDir)

	// 后台执行完整流程
	go func() {
		sigCh := make(chan os.Signal, 1)
		result := ExecuteCommandWithSignal(cmdParts, run.CWD, stdoutPath, stderrPath, 0, sigCh)
		s.store.UpdateRunStatus(newID, result.Status, result.ExitCode, result.EndedAt, result.DurationMs)
		s.store.UpdateRunResources(newID, result.PeakRSSKB, result.CPUTimeMs)
	}()

	jsonResponse(w, map[string]any{"ok": true, "run_id": newID})
}

func (s *WebServer) apiKill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	internal.Log().Info("api_kill", "run_id", id, "remote", r.RemoteAddr)
	run, err := s.store.GetRun(id)
	if err != nil {
		httpError(w, err.Error(), 404)
		return
	}
	if run.Status != "running" {
		httpError(w, "只能终止运行中的任务", 400)
		return
	}

	pidFile := filepath.Join(run.RunDir, ".pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		httpError(w, "找不到进程信息（可能已结束）", 404)
		return
	}

	var pid int
	fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid)
	if pid <= 0 {
		httpError(w, "无效的 PID", 500)
		return
	}

	// 探活：signal 0 不发送实际信号，仅检查进程是否存在
	if err := syscall.Kill(pid, 0); err != nil {
		if err == syscall.ESRCH {
			// 进程已不存在，自动修正状态为 failed
			s.store.UpdateRunStatus(id, "failed", -1, "", 0)
			jsonResponse(w, map[string]any{"ok": true, "msg": fmt.Sprintf("进程 %d 已不存在，已自动标记为 failed", pid)})
			return
		}
		httpError(w, fmt.Sprintf("无法访问进程 %d: %v", pid, err), 500)
		return
	}

	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		syscall.Kill(pid, syscall.SIGKILL)
	}

	// 等待进程退出后采集资源数据
	time.Sleep(500 * time.Millisecond)
	pss, cst := readProcStats(pid)
	if pss > 0 || cst > 0 {
		s.store.UpdateRunResources(id, pss, cst)
	}
	jsonResponse(w, map[string]any{"ok": true, "killed": pid, "msg": "已发送终止信号"})
}

func (s *WebServer) apiDeleteRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	internal.Log().Info("api_delete", "run_id", id, "remote", r.RemoteAddr)
	run, err := s.store.GetRun(id)
	if err != nil {
		httpError(w, err.Error(), 404)
		return
	}
	if run.Status == "running" {
		httpError(w, "请先终止运行中的任务再删除", 400)
		return
	}
	if err := s.store.DeleteRun(id); err != nil {
		httpError(w, "删除失败: "+err.Error(), 500)
		return
	}
	jsonResponse(w, map[string]any{"ok": true, "deleted": id})
}

func (s *WebServer) apiProjects(w http.ResponseWriter, r *http.Request) {
	runs, err := s.store.ListRuns(1000, "", "", "", "", "", "")
	if err != nil {
		httpError(w, err.Error(), 500)
		return
	}
	seen := make(map[string]bool)
	var projects []string
	for _, run := range runs {
		if run.Project != "" && !seen[run.Project] {
			seen[run.Project] = true
			projects = append(projects, run.Project)
		}
	}
	jsonResponse(w, projects)
}

func (s *WebServer) apiTags(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListRuns(1000, "", "", "", "", "", "")
	if err != nil {
		httpError(w, err.Error(), 500)
		return
	}
	seen := make(map[string]bool)
	var tags []string
	for _, row := range rows {
		ts, _ := s.store.GetTags(row.ID)
		for _, t := range ts {
			if !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
	}
	jsonResponse(w, tags)
}

// --- Page handlers ---

func (s *WebServer) pageIndex(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "index.html", nil)
}

func (s *WebServer) pageRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.renderTemplate(w, "run.html", map[string]string{"RunID": id})
}

// --- Helpers ---

func (s *WebServer) renderTemplate(w http.ResponseWriter, name string, data any) {
	dataBytes, err := fs.ReadFile(s.tmplDir, name)
	if err != nil {
		httpError(w, "template not found: "+name, 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(dataBytes)
}

func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func httpError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// autoRefreshInterval 用于 running 状态的日志轮询间隔（毫秒）
const autoRefreshIntervalMs = 3000

// printLANAddrs 输出所有局域网可访问的 IP 地址
func (s *WebServer) printLANAddrs(port int) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	skipPrefixes := []string{"docker", "br-", "veth", "utun", "tun", "wg", "flannel"}
	for _, iface := range ifaces {
		name := iface.Name
		skip := false
		for _, p := range skipPrefixes {
			if strings.HasPrefix(name, p) {
				skip = true
				break
			}
		}
		if !skip && isRandomIfaceName(name) {
			continue
		}
		if skip || iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			fmt.Printf("  [web] http://%s:%d  (%s)\n", ip.String(), port, name)
			internal.Log().Info("web_lan_addr", "url", fmt.Sprintf("http://%s:%d", ip.String(), port), "iface", name)
		}
	}
}

// isRandomIfaceName 检测接口名是否像随机生成的 ID（Docker 容器网卡等）
func isRandomIfaceName(name string) bool {
	for _, p := range []string{"en", "eth", "wl", "wlan"} {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	if len(name) < 8 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// --- Health Check ---

func (s *WebServer) healthCheckLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		s.checkRunningTasks()
	}
}

func (s *WebServer) checkRunningTasks() {
	runs, err := s.store.ListRuns(200, "", "running", "", "", "", "")
	if err != nil {
		internal.Log().Error("health_check_query_failed", "error", err.Error())
		return
	}

	if len(runs) == 0 {
		return
	}

	internal.Log().Info("health_check", "running_count", len(runs))

	for _, run := range runs {
		pid, ok := s.readPID(run.RunDir)
		if !ok {
			s.store.UpdateRunStatus(run.ID, "failed", -1, "", 0)
			internal.Log().Warn("health_check_zombie_no_pid", "run_id", run.ID)
			continue
		}

		if err := syscall.Kill(pid, 0); err != nil {
			s.store.UpdateRunStatus(run.ID, "failed", -1, "", 0)
			internal.Log().Warn("health_check_zombie_process_dead", "run_id", run.ID, "pid", pid)
		}
	}
}

func (s *WebServer) readPID(runDir string) (int, bool) {
	data, err := os.ReadFile(filepath.Join(runDir, ".pid"))
	if err != nil {
		return 0, false
	}
	var pid int
	if n, _ := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); n == 1 && pid > 0 {
		return pid, true
	}
	return 0, false
}

// readInputScript 读取 run 目录下保存的输入脚本快照
func readInputScript(runDir string) string {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "script.") {
			data, err := os.ReadFile(filepath.Join(runDir, e.Name()))
			if err != nil {
				return ""
			}
			return string(data)
		}
	}
	return ""
}
