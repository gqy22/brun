package cmd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"strconv"
	"strings"

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
	mux.HandleFunc("GET /api/runs/{id}/artifacts", s.apiGetArtifacts)
	mux.HandleFunc("POST /api/runs/{id}/rerun", s.apiRerun)
	mux.HandleFunc("POST /api/runs/{id}/kill", s.apiKill)
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
			fmt.Printf("[web] 端口 %d 被占用，自动使用 %d\n", s.port, p)
		}
		fmt.Printf("[web] brun dashboard: http://%s\n", addr)
		s.printLANAddrs(p)
		srv := &http.Server{Handler: mux}
		return srv.Serve(ln)
	}
}

// --- JSON API handlers ---

func (s *WebServer) apiListRuns(w http.ResponseWriter, r *http.Request) {
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

	content := string(data)
	if tailN > 0 {
		content = TailLog(content, tailN)
	}

	jsonResponse(w, map[string]any{
		"content": content,
		"stream":  stream,
		"size":    len(data),
	})
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

	c := exec.CommandContext(r.Context(), cmdParts[0], cmdParts[1:]...)
	c.Dir = run.CWD
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Start(); err != nil {
		httpError(w, "rerun failed: "+err.Error(), 500)
		return
	}

	jsonResponse(w, map[string]any{"ok": true, "pid": c.Process.Pid, "cmd": run.Command})
}

func (s *WebServer) apiKill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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

	p, err := os.FindProcess(pid)
	if err != nil {
		httpError(w, fmt.Sprintf("进程 %d 不存在（可能已结束）", pid), 410)
		return
	}

	if err := p.Signal(syscall.SIGTERM); err != nil {
		p.Signal(syscall.SIGKILL)
	}

	jsonResponse(w, map[string]any{"ok": true, "killed": pid, "msg": "已发送终止信号"})
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
