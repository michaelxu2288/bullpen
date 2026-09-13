package crew

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Store struct {
	filePath string
}

func NewStore() (*Store, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to read user config dir: %w", err)
	}

	root := filepath.Join(configDir, "bullpen")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create config root: %w", err)
	}

	path := filepath.Join(root, "sessions.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
			return nil, fmt.Errorf("failed to initialize session store: %w", err)
		}
	}

	return &Store{filePath: path}, nil
}

func (s *Store) Load() ([]Session, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read session store: %w", err)
	}
	var sessions []Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, fmt.Errorf("failed to parse session store: %w", err)
	}
	return sessions, nil
}

func (s *Store) Save(sessions []Session) error {
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sessions: %w", err)
	}
	return os.WriteFile(s.filePath, data, 0o644)
}

func (s *Store) Upsert(session Session) error {
	sessions, err := s.Load()
	if err != nil {
		return err
	}

	replaced := false
	for i := range sessions {
		if sessions[i].Name == session.Name {
			sessions[i] = session
			replaced = true
			break
		}
	}
	if !replaced {
		sessions = append(sessions, session)
	}

	return s.Save(sessions)
}

func (s *Store) DeleteByName(name string) error {
	sessions, err := s.Load()
	if err != nil {
		return err
	}
	filtered := make([]Session, 0, len(sessions))
	for _, sess := range sessions {
		if sess.Name != name {
			filtered = append(filtered, sess)
		}
	}
	return s.Save(filtered)
}
