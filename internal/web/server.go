package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/availhealth/archon/internal/approval"
	"github.com/availhealth/archon/internal/config"
	"github.com/availhealth/archon/internal/db"
	gh "github.com/availhealth/archon/internal/github"
	"github.com/availhealth/archon/internal/jira"
	"github.com/availhealth/archon/internal/logx"
	"github.com/availhealth/archon/internal/sessionlog"
)

//go:embed templates/*.html
var templateFS embed.FS

type Server struct {
	cfg       config.Config
	log       *slog.Logger
	store     *db.Store
	approval  *approval.Service
	jira      *jira.Client
	github    *gh.Client
	logs      *sessionlog.Hub
	http      *http.Server
	templates *template.Template
	startedAt time.Time
}

type healthResponse struct {
	Status       string            `json:"status"`
	Dependencies map[string]string `json:"dependencies"`
}

type IndexData struct {
	Mode      string
	Project   string
	Repo      string
	StartedAt time.Time
	Config    string
	Sessions  []db.SessionSummary
}

type SessionPageData struct {
	Mode    string
	Session db.SessionDetail
	Error   string
}

func New(cfg config.Config, logger *slog.Logger, store *db.Store, approvalService *approval.Service, jiraClient *jira.Client, githubClient *gh.Client, logHub *sessionlog.Hub) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	s := &Server{
		cfg:       cfg,
		log:       logx.WithComponent(logger, "web"),
		store:     store,
		approval:  approvalService,
		jira:      jiraClient,
		github:    githubClient,
		logs:      logHub,
		templates: tmpl,
		startedAt: time.Now().UTC(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /sessions/{issueKey}", s.handleSession)
	mux.HandleFunc("POST /sessions/{issueKey}/approve", s.handleApprove)
	mux.HandleFunc("POST /sessions/{issueKey}/reject", s.handleReject)
	mux.HandleFunc("GET /sessions/{issueKey}/logs", s.handleSessionLogs)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /health/ready", s.handleReady)
	mux.HandleFunc("GET /metrics", s.handleMetrics)

	s.http = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.UI.Host, cfg.UI.Port),
		Handler:           s.requestLogging(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s, nil
}

func (s *Server) Start() error {
	s.log.Info("starting web server", slog.String("addr", s.http.Addr))
	err := s.http.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	sessions, err := s.store.ListSessions(ctx, 20)
	if err != nil {
		s.log.Error("list sessions", slog.Any("error", err))
		http.Error(w, "failed to load sessions", http.StatusInternalServerError)
		return
	}

	data := IndexData{
		Mode:      s.cfg.Mode,
		Project:   s.cfg.Jira.Projects[0].Key,
		Repo:      s.cfg.GitHub.Repo,
		StartedAt: s.startedAt,
		Config:    s.cfg.ConfigFile,
		Sessions:  sessions,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		s.log.Error("render index", slog.Any("error", err))
		http.Error(w, "failed to render page", http.StatusInternalServerError)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	response := s.collectHealth(ctx)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	response := s.collectHealth(ctx)
	w.Header().Set("Content-Type", "application/json")
	if response.Status != "ready" {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	issueKey := strings.TrimSpace(r.PathValue("issueKey"))
	if issueKey == "" {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	detail, err := s.store.LoadSessionDetail(ctx, issueKey)
	if err != nil {
		s.log.Error("load session detail", slog.String("issue_key", issueKey), slog.Any("error", err))
		http.Error(w, "failed to load session", http.StatusInternalServerError)
		return
	}

	data := SessionPageData{
		Mode:    s.cfg.Mode,
		Session: detail,
		Error:   r.URL.Query().Get("error"),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "session.html", data); err != nil {
		s.log.Error("render session", slog.Any("error", err))
		http.Error(w, "failed to render page", http.StatusInternalServerError)
	}
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	issueKey := strings.TrimSpace(r.PathValue("issueKey"))
	if issueKey == "" {
		http.NotFound(w, r)
		return
	}
	version, err := parseVersion(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.approval.Approve(r.Context(), issueKey, version); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/sessions/%s?error=%s", issueKey, url.QueryEscape(err.Error())), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/sessions/%s", issueKey), http.StatusSeeOther)
}

func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	issueKey := strings.TrimSpace(r.PathValue("issueKey"))
	if issueKey == "" {
		http.NotFound(w, r)
		return
	}
	version, err := parseVersion(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	if err := s.approval.Reject(r.Context(), issueKey, version, reason); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/sessions/%s?error=%s", issueKey, url.QueryEscape(err.Error())), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/sessions/%s", issueKey), http.StatusSeeOther)
}

func (s *Server) handleSessionLogs(w http.ResponseWriter, r *http.Request) {
	issueKey := strings.TrimSpace(r.PathValue("issueKey"))
	if issueKey == "" {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	snapshot, ch, unsubscribe := s.logs.Subscribe(issueKey)
	defer unsubscribe()
	for _, entry := range snapshot {
		if err := writeSSEEntry(w, entry); err != nil {
			return
		}
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSEEntry(w, entry); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) requestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := fmt.Sprintf("%d", time.Now().UnixNano())
		ctx := logx.ContextWithRequestID(r.Context(), requestID)
		logger := logx.WithContext(ctx, s.log)
		start := time.Now()
		next.ServeHTTP(w, r.WithContext(ctx))
		logger.Info("request complete",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

func parseVersion(r *http.Request) (int, error) {
	if err := r.ParseForm(); err != nil {
		return 0, fmt.Errorf("parse form: %w", err)
	}
	value := strings.TrimSpace(r.FormValue("version"))
	if value == "" {
		return 0, fmt.Errorf("missing session version")
	}
	version, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid session version")
	}
	return version, nil
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	snapshot, err := s.store.MetricsSnapshot(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("metrics unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprintf(w, "archon_sessions_total %d\n", snapshot.SessionsTotal)
	for _, state := range db.SortedStates(snapshot.SessionsByState) {
		_, _ = fmt.Fprintf(w, "archon_sessions_by_state{state=%q} %d\n", state, snapshot.SessionsByState[state])
	}
	_, _ = fmt.Fprintf(w, "archon_evaluation_results_total %d\n", snapshot.EvaluationResultsTotal)
	_, _ = fmt.Fprintf(w, "archon_clarification_cycles_total %d\n", snapshot.ClarificationCyclesTotal)
	_, _ = fmt.Fprintf(w, "archon_opencode_runs_total %d\n", snapshot.OpencodeRunsTotal)
	_, _ = fmt.Fprintf(w, "archon_opencode_runs_success_total %d\n", snapshot.OpencodeRunsSuccess)
	_, _ = fmt.Fprintf(w, "archon_worktrees_total %d\n", snapshot.WorktreesTotal)
}

func (s *Server) collectHealth(ctx context.Context) healthResponse {
	deps := map[string]string{}
	status := "ready"
	if err := s.store.Ping(ctx); err != nil {
		deps["database"] = err.Error()
		status = "degraded"
	} else {
		deps["database"] = "ok"
	}
	if err := checkDocker(ctx); err != nil {
		deps["docker"] = err.Error()
		status = "degraded"
	} else {
		deps["docker"] = "ok"
	}
	if err := s.jira.Check(ctx); err != nil {
		deps["jira"] = err.Error()
		status = "degraded"
	} else {
		deps["jira"] = "ok"
	}
	if err := s.github.Check(ctx); err != nil {
		deps["github"] = err.Error()
		status = "degraded"
	} else {
		deps["github"] = "ok"
	}
	return healthResponse{Status: status, Dependencies: deps}
}

func checkDocker(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "info")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker unavailable: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func writeSSEEntry(w http.ResponseWriter, entry sessionlog.Entry) error {
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: log\ndata: %s\n\n", payload)
	return err
}
