package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Command struct {
	Name string
	Args []string
}

type Result struct {
	Command string
	Output  string
}

func Detect(repoPath string) ([]Command, error) {
	if exists(filepath.Join(repoPath, "go.mod")) || exists(filepath.Join(repoPath, "go.work")) {
		return []Command{{Name: "go", Args: []string{"test", "./..."}}}, nil
	}
	if exists(filepath.Join(repoPath, "package.json")) {
		hasTest, manager, err := nodeTestCommand(repoPath)
		if err != nil {
			return nil, err
		}
		if hasTest {
			switch manager {
			case "pnpm":
				return []Command{{Name: "pnpm", Args: []string{"test", "--if-present"}}}, nil
			case "yarn":
				return []Command{{Name: "yarn", Args: []string{"test"}}}, nil
			default:
				return []Command{{Name: "npm", Args: []string{"test", "--", "--runInBand"}}}, nil
			}
		}
	}
	return nil, fmt.Errorf("could not detect verification command for repo")
}

func Run(ctx context.Context, worktreePath string, commands []Command) ([]Result, error) {
	results := make([]Result, 0, len(commands))
	for _, command := range commands {
		cmd := exec.CommandContext(ctx, command.Name, command.Args...)
		cmd.Dir = worktreePath
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			output := strings.TrimSpace(strings.Join([]string{stdout.String(), stderr.String()}, "\n"))
			return results, fmt.Errorf("verification failed for %s %s: %w (%s)", command.Name, strings.Join(command.Args, " "), err, output)
		}
		results = append(results, Result{Command: command.Name + " " + strings.Join(command.Args, " "), Output: strings.TrimSpace(strings.Join([]string{stdout.String(), stderr.String()}, "\n"))})
	}
	return results, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func nodeTestCommand(repoPath string) (bool, string, error) {
	data, err := os.ReadFile(filepath.Join(repoPath, "package.json"))
	if err != nil {
		return false, "", fmt.Errorf("read package.json: %w", err)
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false, "", fmt.Errorf("parse package.json: %w", err)
	}
	manager := "npm"
	switch {
	case exists(filepath.Join(repoPath, "pnpm-lock.yaml")):
		manager = "pnpm"
	case exists(filepath.Join(repoPath, "yarn.lock")):
		manager = "yarn"
	}
	_, hasTest := pkg.Scripts["test"]
	return hasTest, manager, nil
}
