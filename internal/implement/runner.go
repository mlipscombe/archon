package implement

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/availhealth/archon/internal/config"
)

type Result struct {
	PromptSHA    string
	Stdout       string
	StartedAt    time.Time
	FinishedAt   time.Time
	Summary      string
	FilesChanged []string
}

type outputPayload struct {
	Task         string   `json:"task"`
	Status       string   `json:"status"`
	Summary      string   `json:"summary"`
	FilesChanged []string `json:"files_changed"`
}

func Run(ctx context.Context, cfg config.Config, worktreePath, prompt string) (Result, error) {
	return RunWithLogger(ctx, cfg, worktreePath, prompt, nil)
}

func RunWithLogger(ctx context.Context, cfg config.Config, worktreePath, prompt string, onLine func(stream, line string)) (Result, error) {
	startedAt := time.Now().UTC()
	hash := sha256.Sum256([]byte(prompt))
	promptDir, err := os.MkdirTemp("", "archon-impl-*")
	if err != nil {
		return Result{}, fmt.Errorf("create implementation temp dir: %w", err)
	}
	defer os.RemoveAll(promptDir)
	promptFile := filepath.Join(promptDir, "prompt.txt")
	if err := os.WriteFile(promptFile, []byte(prompt), 0o600); err != nil {
		return Result{}, fmt.Errorf("write implementation prompt file: %w", err)
	}

	cmd := exec.CommandContext(ctx,
		"docker", "run", "--rm",
		"--network", cfg.Sandbox.Network,
		"-v", fmt.Sprintf("%s:/workspace", worktreePath),
		"-v", fmt.Sprintf("%s:/archon-tmp:ro", promptDir),
		"-w", "/workspace",
		"-e", `OPENCODE_PERMISSION={"*":"allow"}`,
		"-e", "ARCHON_SESSION_TASK=implementation",
		cfg.Sandbox.Image,
		"sh", "-lc", shellCommand(cfg.Opencode.BinaryPath),
	)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("open implementation stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, fmt.Errorf("open implementation stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start opencode implementation: %w", err)
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		streamPipe(stdoutPipe, &stdoutBuf, "stdout", onLine)
	}()
	go func() {
		defer wg.Done()
		streamPipe(stderrPipe, &stderrBuf, "stderr", onLine)
	}()
	err = cmd.Wait()
	wg.Wait()
	finishedAt := time.Now().UTC()
	combinedOutput := strings.TrimSpace(strings.Join([]string{stdoutBuf.String(), stderrBuf.String()}, "\n"))
	result := Result{PromptSHA: hex.EncodeToString(hash[:]), Stdout: combinedOutput, StartedAt: startedAt, FinishedAt: finishedAt}
	if err != nil {
		return result, fmt.Errorf("run opencode implementation: %w", err)
	}
	payload, err := parseOutput(result.Stdout)
	if err != nil {
		return result, err
	}
	if strings.ToLower(strings.TrimSpace(payload.Status)) != "success" {
		return result, fmt.Errorf("implementation reported failure")
	}
	result.Summary = payload.Summary
	result.FilesChanged = payload.FilesChanged
	return result, nil
}

func shellCommand(binaryPath string) string {
	binary := strconv.Quote(binaryPath)
	return fmt.Sprintf("PROMPT=$(cat /archon-tmp/prompt.txt); %s run --format json \"$PROMPT\"", binary)
}

func parseOutput(output string) (outputPayload, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return outputPayload{}, fmt.Errorf("empty implementation output")
	}
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var payload outputPayload
		if err := json.Unmarshal([]byte(line), &payload); err == nil && strings.TrimSpace(payload.Status) != "" {
			return payload, nil
		}
	}
	var payload outputPayload
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return outputPayload{}, fmt.Errorf("parse implementation output: %w", err)
	}
	return payload, nil
}

func streamPipe(reader io.Reader, buffer *bytes.Buffer, stream string, onLine func(stream, line string)) {
	tee := io.TeeReader(reader, buffer)
	scanner := bufio.NewScanner(tee)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if onLine != nil {
			onLine(stream, line)
		}
	}
}
