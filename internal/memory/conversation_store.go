package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Message struct {
	Session   string    `json:"session"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type ConversationStore struct {
	path string
}

func NewConversationStore() (*ConversationStore, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache dir: %w", err)
	}
	root := filepath.Join(cacheDir, "bullpen")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache root: %w", err)
	}
	p := filepath.Join(root, "conversations.json")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		_ = os.WriteFile(p, []byte("[]"), 0o644)
	}
	return &ConversationStore{path: p}, nil
}

func (s *ConversationStore) Append(m Message) error {
	all, err := s.All()
	if err != nil {
		return err
	}
	all = append(all, m)
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

func (s *ConversationStore) All() ([]Message, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var out []Message
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
