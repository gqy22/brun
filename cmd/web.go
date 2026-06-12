package cmd

import (
	"encoding/json"
	"fmt"
	"io"
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

const processActivitySampleInterval = 300 * time.Millisecond

type RunProcessSummary struct {
	RootPID          int    `json:"root_pid"`
	ProcessCount     int    `json:"process_count"`
	ActiveCount      int    `json:"active_count"`
	TotalRSSKB       int64  `json:"total_rss_kb"`
	LastLogUpdate    string `json:"last_log_update"`
	LastLogUpdateAgo string `json:"last_log_update_ago"`
	LastLogStatus    string `json:"last_log_status"`
	ProcessSource    string `json:"process_source"`
	ActivitySampled  bool   `json:"activity_sampled"`
}

type WebServer struct {
	store         *internal.Store
	addr          string
	port          int
	autoIncrement bool
	tmplDir       fs.FS
	static        fs.FS
}

func NewWebServer(store *internal.Store, addr string, port int, tmplFS, staticFS fs.FS) *WebServer {
	return &WebServer{
		store:         store,
		addr:          addr,
		port:          port,
		autoIncrement: true,
		tmplDir:       tmplFS,
		static:        staticFS,
	}
}

func (s *WebServer) SetAutoIncrementPort(enabled bool) {
	s.autoIncrement = enabled
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
	mux.HandleFunc("GET /api/runs/{id}/processes", s.apiGetProcesses)
	mux.HandleFunc("POST /api/runs/{id}/rerun", s.apiRerun)
	mux.HandleFunc("POST /api/runs/{id}/kill", s.apiKill)
	mux.HandleFunc("DELETE /api/runs/{id}", s.apiDeleteRun)
	mux.HandleFunc("GET /api/projects", s.apiProjects)
	mux.HandleFunc("GET /api/tags", s.apiTags)
	mux.HandleFunc("GET /api/hosts", s.apiHosts)
	mux.HandleFunc("GET /api/users", s.apiUsers)

	// Page routes (catch-all last)
	mux.HandleFunc("GET /", s.pageIndex)
	mux.HandleFunc("GET /run/{id}", s.pageRun)

	for attempt := 0; ; attempt++ {
		p := s.port + attempt
		addr := fmt.Sprintf("%s:%d", s.addr, p)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			if s.autoIncrement && attempt < 20 {
				continue
			}
			if s.autoIncrement {
				return fmt.Errorf("端口 %d-%d 均被占用，请手动指定 --port", s.port, p-1)
			}
			return fmt.Errorf("端口 %d 不可用: %w", s.port, err)
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
	displayStatus := r.URL.Query().Get("display_status")
	tag := r.URL.Query().Get("tag")
	search := r.URL.Query().Get("search")
	host := r.URL.Query().Get("host")
	user := r.URL.Query().Get("user")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	// display_status=success_with_warnings 等：翻译成 status + diag_warning_count>0 的 DB 过滤。
	withWarnings := false
	if displayStatus != "" {
		switch displayStatus {
		case "success_with_warnings", "failed_with_warnings", "cancelled_with_warnings":
			withWarnings = true
			if status == "" {
				status = strings.TrimSuffix(displayStatus, "_with_warnings")
			}
		default:
			// 未知值按字面 status 处理（与原行为兼容）
			if status == "" {
				status = displayStatus
			}
		}
	}

	runs, err := s.store.ListRuns(limit, project, status, tag, search, "", "", withWarnings, host, user)
	if err != nil {
		httpError(w, err.Error(), 500)
		return
	}

	type runRow struct {
		ID                string                     `json:"id"`
		Name              string                     `json:"name"`
		Project           string                     `json:"project"`
		Status            string                     `json:"status"`
		DisplayStatus     string                     `json:"display_status"`
		Duration          string                     `json:"duration"`
		Command           string                     `json:"command"`
		StartedAt         string                     `json:"started_at"`
		DiagnosticSummary internal.DiagnosticSummary `json:"diagnostic_summary"`
	}

	rows := make([]runRow, len(runs))
	for i, run := range runs {
		rows[i] = runRow{
			ID:                run.ID,
			Name:              run.Name,
			Project:           run.Project,
			Status:            run.Status,
			DisplayStatus:     run.DisplayStatus(),
			Duration:          DisplayDuration(run.Status, run.StartedAt, run.DurationMs),
			Command:           truncate(run.Command, 80),
			StartedAt:         run.StartedAt,
			DiagnosticSummary: internal.DiagnosticSummaryFromRun(run),
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
	script, _ := ReadScriptSnapshot(run.RunDir)
	diagnostics, err := internal.ReadDiagnostics(run.RunDir)
	if err != nil {
		internal.Log().Warn("diagnostics_read_failed", "run_id", run.ID, "error", err.Error())
		diagnostics = nil
	}
	diagnosticSummary := internal.SummarizeDiagnostics(diagnostics)
	if len(diagnostics) == 0 {
		diagnosticSummary = internal.DiagnosticSummaryFromRun(run)
	}
	processSummary := RunProcessSummary{}
	if run.Status == "running" {
		processSummary = s.buildRunProcessSummary(run)
	}

	jsonResponse(w, map[string]any{
		"id":                 run.ID,
		"name":               run.Name,
		"project":            run.Project,
		"project_source":     run.ProjectSource,
		"cwd":                run.CWD,
		"cwd_source":         run.CWDSource,
		"command":            run.Command,
		"script":             script.Content,
		"script_name":        script.Name,
		"status":             run.Status,
		"display_status":     run.DisplayStatus(),
		"exit_code":          run.ExitCode,
		"started_at":         run.StartedAt,
		"ended_at":           run.EndedAt,
		"duration_ms":        run.DurationMs,
		"duration":           DisplayDuration(run.Status, run.StartedAt, run.DurationMs),
		"hostname":           run.Hostname,
		"hostname_status":    run.HostnameStatus,
		"username":           run.Username,
		"username_status":    run.UsernameStatus,
		"git_repo":           run.GitRepo,
		"git_branch":         run.GitBranch,
		"git_commit":         run.GitCommit,
		"git_dirty":          run.GitDirty,
		"conda_status":       run.CondaStatus,
		"conda_env":          run.CondaEnv,
		"conda_prefix":       run.CondaPrefix,
		"python_version":     run.PythonVersion,
		"resource_supported": run.ResourceSupported,
		"resource_status":    run.ResourceStatus,
		"peak_rss_kb":        run.PeakRSSKB,
		"cpu_time_ms":        run.CPUTimeMs,
		"run_dir":            run.RunDir,
		"tags":               tags,
		"note":               note,
		"diagnostics":        diagnostics,
		"diagnostic_summary": diagnosticSummary,
		"process_summary":    processSummary,
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

	fileSize, data, err := readLogFromOffset(logPath, int64(offset))
	if err != nil {
		status := "unreadable"
		if os.IsNotExist(err) {
			status = "missing"
		}
		jsonResponse(w, map[string]any{
			"content": "",
			"stream":  stream,
			"status":  status,
			"size":    0,
		})
		return
	}

	if offset > 0 && int64(offset) >= fileSize {
		jsonResponse(w, map[string]any{
			"content": "",
			"stream":  stream,
			"status":  "ok",
			"size":    fileSize,
		})
		return
	}

	content := string(data)
	if tailN > 0 {
		content = TailLog(content, tailN)
	}

	jsonResponse(w, map[string]any{
		"content": content,
		"stream":  stream,
		"status":  "ok",
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

	fileSize, data, readErr := readLogFromOffset(logPath, 0)
	if readErr != nil {
		data = []byte{}
		fileSize = 0
	}
	prevSize := fileSize

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

			fileSize, data, readErr = readLogFromOffset(logPath, prevSize)
			if readErr != nil {
				continue
			}
			if len(data) > 0 {
				sendSSE(map[string]any{
					"content": string(data),
					"size":    fileSize,
				})
				prevSize = fileSize
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

func (s *WebServer) apiGetProcesses(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.store.GetRun(id)
	if err != nil {
		httpError(w, err.Error(), 404)
		return
	}
	if run.Status != "running" {
		jsonResponse(w, map[string]any{
			"processes": []ProcessInfo{},
			"status":    run.Status,
			"summary":   RunProcessSummary{},
		})
		return
	}

	pid, ok := s.readPID(run.RunDir)
	if !ok || pid <= 0 {
		jsonResponse(w, map[string]any{
			"processes": []ProcessInfo{},
			"summary":   RunProcessSummary{},
		})
		return
	}

	procs, processSource, activitySampled := collectProcesses(pid, processActivitySampleInterval)
	summary := summarizeProcesses(pid, run.RunDir, procs, processSource, activitySampled)
	jsonResponse(w, map[string]any{
		"processes":        procs,
		"total_rss_kb":     summary.TotalRSSKB,
		"count":            len(procs),
		"process_source":   summary.ProcessSource,
		"activity_sampled": summary.ActivitySampled,
		"summary":          summary,
	})
}

func (s *WebServer) apiRerun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.store.GetRun(id)
	if err != nil {
		httpError(w, err.Error(), 404)
		return
	}

	if strings.TrimSpace(run.Command) == "" {
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
		result := ExecuteCommandWithSignal(ShellCommandArgs(run.Command), run.CWD, stdoutPath, stderrPath, 0, sigCh)
		s.store.UpdateRunStatus(newID, result.Status, result.ExitCode, result.EndedAt, result.DurationMs)
		s.store.UpdateRunResources(newID, result.PeakRSSKB, result.CPUTimeMs, result.ResourceSupported, result.ResourceStatus)
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

	// 记录最终资源数据
	pss, cst := ReadProcStats(pid)
	if pss > 0 || cst > 0 {
		s.store.UpdateRunResources(id, pss, cst, ResourceSupported(), "ok")
	}

	// 调用统一的 StopRun，3 秒宽限期
	result := StopRun(pid, 3, false)

	if result.AlreadyDead {
		s.store.UpdateRunStatus(id, "failed", -1, "", 0)
	}

	if !result.OK {
		httpError(w, result.Msg, 500)
		return
	}

	jsonResponse(w, map[string]any{"ok": true, "killed": result.PID, "msg": result.Msg})
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
	runs, err := s.store.ListRuns(1000, "", "", "", "", "", "", false, "", "")
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
	rows, err := s.store.ListRuns(1000, "", "", "", "", "", "", false, "", "")
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

// apiHosts 列出所有出现过的 hostname（含空值跳过），用于 Web 端 host 过滤的 datalist 建议。
func (s *WebServer) apiHosts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListRuns(1000, "", "", "", "", "", "", false, "", "")
	if err != nil {
		httpError(w, err.Error(), 500)
		return
	}
	seen := make(map[string]bool)
	var hosts []string
	for _, run := range rows {
		if run.Hostname != "" && !seen[run.Hostname] {
			seen[run.Hostname] = true
			hosts = append(hosts, run.Hostname)
		}
	}
	sortStrings(hosts)
	jsonResponse(w, hosts)
}

// apiUsers 列出所有出现过的 username，用于 Web 端 user 过滤的 datalist 建议。
func (s *WebServer) apiUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListRuns(1000, "", "", "", "", "", "", false, "", "")
	if err != nil {
		httpError(w, err.Error(), 500)
		return
	}
	seen := make(map[string]bool)
	var users []string
	for _, run := range rows {
		if run.Username != "" && !seen[run.Username] {
			seen[run.Username] = true
			users = append(users, run.Username)
		}
	}
	sortStrings(users)
	jsonResponse(w, users)
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
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

func readLogFromOffset(path string, offset int64) (int64, []byte, error) {
	if offset < 0 {
		offset = 0
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, nil, err
	}
	size := info.Size()
	if offset >= size {
		return size, []byte{}, nil
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return size, nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return size, nil, err
	}
	return size, data, nil
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
	runs, err := s.store.ListRuns(200, "", "running", "", "", "", "", false, "", "")
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
			s.markRunFailedNow(run)
			internal.Log().Warn("health_check_zombie_no_pid", "run_id", run.ID)
			continue
		}

		if err := syscall.Kill(pid, 0); err != nil {
			s.markRunFailedNow(run)
			internal.Log().Warn("health_check_zombie_process_dead", "run_id", run.ID, "pid", pid)
		}
	}
}

func (s *WebServer) markRunFailedNow(run *internal.Run) {
	endedAt := time.Now().UTC()
	durationMs := run.DurationMs
	if run.StartedAt != "" {
		if started, err := time.Parse(time.RFC3339, run.StartedAt); err == nil {
			durationMs = endedAt.Sub(started).Milliseconds()
			if durationMs < 0 {
				durationMs = 0
			}
		}
	}
	if err := s.store.UpdateRunStatus(run.ID, "failed", -1, endedAt.Format(time.RFC3339), durationMs); err != nil {
		internal.Log().Error("health_check_update_failed", "run_id", run.ID, "error", err.Error())
	}
}

func isActiveProcessState(state string) bool {
	switch state {
	case "R", "D":
		return true
	default:
		return false
	}
}

func readLastLogUpdate(runDir string) (string, string, string) {
	paths := []string{
		filepath.Join(runDir, "stdout.o"),
		filepath.Join(runDir, "stderr.er"),
	}
	var latest time.Time
	sawUnreadable := false
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			if !os.IsNotExist(err) {
				sawUnreadable = true
			}
			continue
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	if latest.IsZero() {
		if sawUnreadable {
			return "", "", "unreadable"
		}
		return "", "", "missing"
	}
	return latest.UTC().Format(time.RFC3339), humanizeAgo(time.Since(latest)), "ok"
}

func humanizeAgo(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func collectProcesses(rootPID int, interval time.Duration) ([]ProcessInfo, string, bool) {
	procs := ListProcessTreeWithActivity(rootPID, interval)
	if len(procs) > 0 {
		return procs, "tree", true
	}
	procs = ListProcessGroup(rootPID)
	if len(procs) > 0 {
		return procs, "group", false
	}
	return []ProcessInfo{}, "empty", false
}

func summarizeProcesses(rootPID int, runDir string, procs []ProcessInfo, processSource string, activitySampled bool) RunProcessSummary {
	totalRSS := int64(0)
	activeCount := 0
	for _, p := range procs {
		totalRSS += p.RSSKB
		if p.IsActive || isActiveProcessState(p.State) {
			activeCount++
		}
	}
	lastLogUpdate, lastLogUpdateAgo, lastLogStatus := readLastLogUpdate(runDir)
	return RunProcessSummary{
		RootPID:          rootPID,
		ProcessCount:     len(procs),
		ActiveCount:      activeCount,
		TotalRSSKB:       totalRSS,
		LastLogUpdate:    lastLogUpdate,
		LastLogUpdateAgo: lastLogUpdateAgo,
		LastLogStatus:    lastLogStatus,
		ProcessSource:    processSource,
		ActivitySampled:  activitySampled,
	}
}

func (s *WebServer) buildRunProcessSummary(run *internal.Run) RunProcessSummary {
	pid, ok := s.readPID(run.RunDir)
	if !ok || pid <= 0 {
		return RunProcessSummary{}
	}
	procs, processSource, activitySampled := collectProcesses(pid, processActivitySampleInterval)
	return summarizeProcesses(pid, run.RunDir, procs, processSource, activitySampled)
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
