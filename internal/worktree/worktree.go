package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Manager struct {
	RepoPath string
	RootDir  string
}

func NewManager(repoPath string) (*Manager, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to read user config dir: %w", err)
	}

	root := filepath.Join(configDir, "bullpen", "worktrees")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create worktree root: %w", err)
	}

	return &Manager{RepoPath: repoPath, RootDir: root}, nil
}

func (m *Manager) Create(sessionName string, branchPrefix string) (branch string, worktreePath string, err error) {
	if branchPrefix == "" {
		branchPrefix = "agent/"
	}

	branch = sanitizeBranchName(branchPrefix + sessionName)
	worktreePath = filepath.Join(m.RootDir, sanitizeBranchName(sessionName)+"_"+fmt.Sprintf("%x", time.Now().UnixNano()))

	headSHA, err := runGit(m.RepoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve HEAD: %w", err)
	}

	if _, err := runGit(m.RepoPath, "worktree", "add", "-b", branch, worktreePath, strings.TrimSpace(headSHA)); err != nil {
		return "", "", fmt.Errorf("failed to create worktree: %w", err)
	}

	return branch, worktreePath, nil
}

func (m *Manager) Remove(worktreePath, branch string) error {
	var errs []string

	if _, err := runGit(m.RepoPath, "worktree", "remove", "-f", worktreePath); err != nil {
		errs = append(errs, err.Error())
	}
	if _, err := runGit(m.RepoPath, "branch", "-D", branch); err != nil {
		errs = append(errs, err.Error())
	}
	if _, err := runGit(m.RepoPath, "worktree", "prune"); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}
	return nil
}

func runGit(repoPath string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", fullArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func sanitizeBranchName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	re := regexp.MustCompile(`[^a-z0-9\-_/\.]+`)
	s = re.ReplaceAllString(s, "")
	s = strings.Trim(s, "-/")
	if s == "" {
		return "agent"
	}
	return s
}
