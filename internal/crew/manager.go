package crew

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/michaelxu2288/bullpen/internal/worktree"
)

type Manager struct {
	repoPath string
	store    *Store
	worktree *worktree.Manager
}

func NewManager(repoPath string) (*Manager, error) {
	store, err := NewStore()
	if err != nil {
		return nil, err
	}
	wt, err := worktree.NewManager(repoPath)
	if err != nil {
		return nil, err
	}
	return &Manager{repoPath: repoPath, store: store, worktree: wt}, nil
}

func (m *Manager) Launch(opts LaunchOptions) (Session, error) {
	if opts.Name == "" {
		return Session{}, fmt.Errorf("session name is required")
	}
	if opts.Program == "" {
		return Session{}, fmt.Errorf("program is required")
	}

	branch, wtPath, err := m.worktree.Create(opts.Name, opts.BranchPrefix)
	if err != nil {
		return Session{}, err
	}

	tmuxName := sanitizeTmuxName(opts.Name)
	if err := run("tmux", "new-session", "-d", "-s", tmuxName, "-c", wtPath, opts.Program); err != nil {
		_ = m.worktree.Remove(wtPath, branch)
		return Session{}, fmt.Errorf("failed to launch tmux session: %w", err)
	}

	s := Session{
		Name:        opts.Name,
		Provider:    opts.Provider,
		Program:     opts.Program,
		Branch:      branch,
		Worktree:    wtPath,
		TmuxSession: tmuxName,
		CreatedAt:   time.Now(),
	}
	if err := m.store.Upsert(s); err != nil {
		return Session{}, err
	}

	return s, nil
}

func (m *Manager) Kill(name string) error {
	sessions, err := m.store.Load()
	if err != nil {
		return err
	}

	for _, sess := range sessions {
		if sess.Name != name {
			continue
		}

		_ = run("tmux", "kill-session", "-t", sess.TmuxSession)
		if err := m.worktree.Remove(sess.Worktree, sess.Branch); err != nil {
			return err
		}
		return m.store.DeleteByName(name)
	}
	return fmt.Errorf("session not found: %s", name)
}

func (m *Manager) SendPrompt(name, prompt string) error {
	sess, err := m.FindByName(name)
	if err != nil {
		return err
	}
	if prompt == "" {
		return nil
	}
	if err := run("tmux", "send-keys", "-t", sess.TmuxSession, prompt); err != nil {
		return err
	}
	return run("tmux", "send-keys", "-t", sess.TmuxSession, "Enter")
}

func (m *Manager) Capture(name string) (string, error) {
	sess, err := m.FindByName(name)
	if err != nil {
		return "", err
	}
	out, err := output("tmux", "capture-pane", "-p", "-S", "-", "-t", sess.TmuxSession)
	if err != nil {
		return "", err
	}
	return out, nil
}

func (m *Manager) FindByName(name string) (Session, error) {
	sessions, err := m.store.Load()
	if err != nil {
		return Session{}, err
	}
	for _, sess := range sessions {
		if sess.Name == name {
			return sess, nil
		}
	}
	return Session{}, fmt.Errorf("session not found: %s", name)
}

func (m *Manager) List() ([]Session, error) {
	return m.store.Load()
}

func sanitizeTmuxName(name string) string {
	replacer := strings.NewReplacer(" ", "_", ".", "_", "/", "_", "\\", "_")
	return "ccag_" + replacer.Replace(name)
}

func run(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s failed: %s", bin, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

func output(bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s failed: %s", bin, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
