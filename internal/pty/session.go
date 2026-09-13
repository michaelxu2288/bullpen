// Package pty manages interactive agent sessions backed by a real terminal
// multiplexer. The default backend is tmux; the interface is small enough that
// a native creack/pty backend can be dropped in without touching callers.
package pty

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Session is a live, attachable terminal hosting one agent process.
type Session struct {
	Name      string
	Program   string
	Workdir   string
	Backend   string
	CreatedAt time.Time
}

// Backend abstracts the multiplexer (tmux today, PTY-native tomorrow).
type Backend interface {
	Spawn(name, workdir, program string) error
	Send(name, keys string) error
	Capture(name string, lines int) (string, error)
	Kill(name string) error
	List() ([]string, error)
	Attach(name string) *exec.Cmd
}

// TmuxBackend drives detached tmux sessions, one per crew.
type TmuxBackend struct {
	Prefix string
}

func (t TmuxBackend) sess(name string) string {
	if t.Prefix == "" {
		t.Prefix = "bullpen"
	}
	return t.Prefix + "_" + name
}

func (t TmuxBackend) Spawn(name, workdir, program string) error {
	s := t.sess(name)
	args := []string{"new-session", "-d", "-s", s}
	if workdir != "" {
		args = append(args, "-c", workdir)
	}
	if program != "" {
		args = append(args, program)
	}
	return run("tmux", args...)
}

// Send types keys into the session, simulating prompt handoff into the crew.
func (t TmuxBackend) Send(name, keys string) error {
	return run("tmux", "send-keys", "-t", t.sess(name), keys, "Enter")
}

// Capture scrapes the last N lines of the session pane (telemetry / kanban).
func (t TmuxBackend) Capture(name string, lines int) (string, error) {
	out, err := output("tmux", "capture-pane", "-p", "-t", t.sess(name), "-S", fmt.Sprintf("-%d", lines))
	return out, err
}

func (t TmuxBackend) Kill(name string) error {
	return run("tmux", "kill-session", "-t", t.sess(name))
}

func (t TmuxBackend) List() ([]string, error) {
	out, err := output("tmux", "list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, err
	}
	all := strings.Split(strings.TrimSpace(out), "\n")
	pref := t.Prefix
	if pref == "" {
		pref = "bullpen"
	}
	mine := make([]string, 0, len(all))
	for _, s := range all {
		if strings.HasPrefix(s, pref+"_") {
			mine = append(mine, strings.TrimPrefix(s, pref+"_"))
		}
	}
	return mine, nil
}

// Attach returns a command the caller can hand stdin/stdout to for a live PTY.
func (t TmuxBackend) Attach(name string) *exec.Cmd {
	return exec.Command("tmux", "attach-session", "-t", t.sess(name))
}

func run(bin string, args ...string) error {
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%s not on PATH (install tmux): %w", bin, err)
	}
	return exec.Command(bin, args...).Run()
}

func output(bin string, args ...string) (string, error) {
	if _, err := exec.LookPath(bin); err != nil {
		return "", fmt.Errorf("%s not on PATH (install tmux): %w", bin, err)
	}
	b, err := exec.Command(bin, args...).CombinedOutput()
	return string(b), err
}
