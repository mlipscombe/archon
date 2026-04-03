package opencode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/availhealth/archon/internal/config"
)

type Runner struct {
	cfg config.Config
}

type RunResult struct {
	Stdout     string
	Stderr     string
	StartedAt  time.Time
	FinishedAt time.Time
	PromptSHA  string
}

func NewRunner(cfg config.Config) *Runner {
	return &Runner{cfg: cfg}
}

func (r *Runner) RunEvaluation(ctx context.Context, prompt string) (RunResult, error) {
	startedAt := time.Now().UTC()
	promptHash := sha256.Sum256([]byte(prompt))

	promptDir, err := os.MkdirTemp("", "archon-opencode-*")
	if err != nil {
		return RunResult{}, fmt.Errorf("create opencode temp dir: %w", err)
	}
	defer os.RemoveAll(promptDir)

	promptFile := filepath.Join(promptDir, "prompt.txt")
	if err := os.WriteFile(promptFile, []byte(prompt), 0o600); err != nil {
		return RunResult{}, fmt.Errorf("write opencode prompt file: %w", err)
	}

	cmd := exec.CommandContext(ctx,
		"docker", "run", "--rm",
		"--network", r.cfg.Sandbox.Network,
		"-v", fmt.Sprintf("%s:/workspace", r.cfg.Repo.Path),
		"-v", fmt.Sprintf("%s:/archon-tmp:ro", promptDir),
		"-w", "/workspace",
		"-e", `OPENCODE_PERMISSION={"*":"allow"}`,
		"-e", "ARCHON_SESSION_TASK=evaluation",
		r.cfg.Sandbox.Image,
		"sh", "-lc", shellCommand(r.cfg.Opencode.BinaryPath),
	)

	output, err := cmd.CombinedOutput()
	finishedAt := time.Now().UTC()
	result := RunResult{
		Stdout:     string(output),
		Stderr:     "",
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		PromptSHA:  hex.EncodeToString(promptHash[:]),
	}
	if err != nil {
		return result, fmt.Errorf("run opencode evaluation: %w", err)
	}
	return result, nil
}

func shellCommand(binaryPath string) string {
	binary := strconv.Quote(binaryPath)
	return fmt.Sprintf("PROMPT=$(cat /archon-tmp/prompt.txt); %s run --format json \"$PROMPT\"", binary)
}
