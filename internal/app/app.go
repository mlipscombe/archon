package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/availhealth/archon/internal/approval"
	"github.com/availhealth/archon/internal/config"
	"github.com/availhealth/archon/internal/db"
	"github.com/availhealth/archon/internal/evaluator"
	gh "github.com/availhealth/archon/internal/github"
	"github.com/availhealth/archon/internal/implement"
	"github.com/availhealth/archon/internal/jira"
	"github.com/availhealth/archon/internal/logx"
	"github.com/availhealth/archon/internal/revision"
	"github.com/availhealth/archon/internal/sessionlog"
	"github.com/availhealth/archon/internal/watcher"
	"github.com/availhealth/archon/internal/web"
)

func Run(args []string, version string) error {
	if len(args) == 0 {
		return runStart(version)
	}

	switch args[0] {
	case "start":
		return runStart(version)
	case "config":
		return runConfigCommand(args[1:])
	case "auth":
		return runAuthCommand(args[1:])
	case "version":
		fmt.Println(version)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runConfigValidate() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fmt.Printf("config valid\nmode=%s\nproject=%s\nrepo=%s\nstate_store=%s\n", cfg.Mode, cfg.Jira.Projects[0].Key, cfg.GitHub.Repo, cfg.StateStore.Path)
	return nil
}

func runStart(version string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logx.New(cfg.Log.Level, cfg.Log.Format).With(
		slog.String("version", version),
		slog.String("mode", cfg.Mode),
	)

	if err := ensureDocker(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := db.Open(ctx, cfg.StateStore.Path, logx.WithComponent(logger, "db"))
	if err != nil {
		return err
	}
	if err := store.RecoverInterruptedSessions(ctx); err != nil {
		return err
	}
	if err := store.SyncProject(ctx, cfg.Jira.Projects[0]); err != nil {
		return err
	}
	if err := store.SeedDefaultRubric(ctx, cfg.Jira.Projects[0].Key); err != nil {
		return err
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("close database", slog.Any("error", err))
		}
	}()

	jiraClient, err := jira.NewClient(cfg)
	if err != nil {
		return err
	}
	githubClient := gh.NewClient(cfg)
	logHub := sessionlog.NewHub()
	evaluatorService := evaluator.New(cfg, logger, store, jiraClient)
	approvalService := approval.New(logger, store, jiraClient)
	implementService := implement.New(cfg, logger, store, githubClient, jiraClient, logHub)
	revisionService := revision.New(cfg, logger, store, githubClient, jiraClient)

	server, err := web.New(cfg, logger, store, approvalService, jiraClient, githubClient, logHub)
	if err != nil {
		return err
	}
	poller := watcher.New(cfg, logger, store, jiraClient, evaluatorService, githubClient)

	errCh := make(chan error, 4)
	go func() {
		errCh <- server.Start()
	}()
	go func() {
		errCh <- poller.Start(ctx)
	}()
	go func() {
		errCh <- implementService.Start(ctx)
	}()
	go func() {
		errCh <- revisionService.Start(ctx)
	}()

	logger.Info("archon started",
		slog.String("ui_addr", fmt.Sprintf("http://%s:%d", cfg.UI.Host, cfg.UI.Port)),
		slog.String("state_store", cfg.StateStore.Path),
		slog.String("repo_path", cfg.Repo.Path),
	)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown web server: %w", err)
		}
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("web server failed: %w", err)
		}
		return nil
	}
}

func ensureDocker() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker is required for mvp startup: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "info")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker is required for mvp startup: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	return nil
}
