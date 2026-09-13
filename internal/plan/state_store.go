package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/michaelxu2288/bullpen/internal/domain"
)

type Snapshot struct {
	RunID      string        `json:"run_id"`
	Goal       string        `json:"goal"`
	Tasks      []domain.Task `json:"tasks"`
	SavedAt    time.Time     `json:"saved_at"`
	Strategy   string        `json:"strategy"`
}

type StateStore struct {
	Path string
}

func NewStateStore(root string) (*StateStore, error) {
	if root == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user cache dir: %w", err)
		}
		root = filepath.Join(cacheDir, "bullpen")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create state root: %w", err)
	}
	return &StateStore{Path: filepath.Join(root, "runs.json")}, nil
}

func (s *StateStore) Append(snapshot Snapshot) error {
	all, err := s.All()
	if err != nil {
		return err
	}
	all = append(all, snapshot)
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, data, 0o644)
}

func (s *StateStore) All() ([]Snapshot, error) {
	if _, err := os.Stat(s.Path); os.IsNotExist(err) {
		if err := os.WriteFile(s.Path, []byte("[]"), 0o644); err != nil {
			return nil, err
		}
	}
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, err
	}
	var out []Snapshot
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
