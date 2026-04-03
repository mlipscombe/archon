package gitops

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Manager struct {
	repoPath     string
	worktreeRoot string
	baseBranch   string
}

func NewManager(repoPath, stateStorePath, baseBranch string) *Manager {
	root := filepath.Join(filepath.Dir(stateStorePath), "worktrees")
	return &Manager{repoPath: repoPath, worktreeRoot: root, baseBranch: baseBranch}
}

func (m *Manager) PrepareWorktree(ctx context.Context, issueKey, issueSummary, existingBranch string) (string, string, error) {
	if err := os.MkdirAll(m.worktreeRoot, 0o755); err != nil {
		return "", "", fmt.Errorf("create worktree root: %w", err)
	}
	branch := existingBranch
	if strings.TrimSpace(branch) == "" {
		branch = BranchName(issueKey, issueSummary)
	}
	worktreePath := filepath.Join(m.worktreeRoot, fmt.Sprintf("%s-%d", sanitize(issueKey), time.Now().UTC().UnixNano()))

	if strings.TrimSpace(existingBranch) == "" {
		if _, err := m.run(ctx, m.repoPath, "git", "worktree", "add", "-b", branch, worktreePath, m.baseBranch); err != nil {
			return "", "", err
		}
	} else {
		if _, err := m.run(ctx, m.repoPath, "git", "worktree", "add", worktreePath, existingBranch); err != nil {
			return "", "", err
		}
	}
	return worktreePath, branch, nil
}

func (m *Manager) HasChanges(ctx context.Context, worktreePath string) (bool, error) {
	output, err := m.run(ctx, worktreePath, "git", "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

func (m *Manager) CommitAndPush(ctx context.Context, worktreePath, branchName, message string) error {
	if _, err := m.run(ctx, worktreePath, "git", "add", "-A"); err != nil {
		return err
	}
	if _, err := m.run(ctx, worktreePath, "git", "commit", "-m", message); err != nil {
		return err
	}
	if _, err := m.run(ctx, worktreePath, "git", "push", "-u", "origin", branchName); err != nil {
		return err
	}
	return nil
}

func BranchName(issueKey, summary string) string {
	slug := sanitize(strings.ToLower(summary))
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.Trim(slug, "-")
	}
	if slug == "" {
		slug = "work-item"
	}
	return fmt.Sprintf("archon/%s-%s", strings.ToUpper(strings.TrimSpace(issueKey)), slug)
}

func sanitize(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	input = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(input, "-")
	return strings.Trim(input, "-")
}

func (m *Manager) run(ctx context.Context, workdir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workdir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		combined := strings.TrimSpace(strings.Join([]string{stdout.String(), stderr.String()}, "\n"))
		return stdout.String(), fmt.Errorf("%s %s failed: %w (%s)", name, strings.Join(args, " "), err, combined)
	}
	return stdout.String(), nil
}
