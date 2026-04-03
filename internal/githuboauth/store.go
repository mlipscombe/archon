package githuboauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrTokenNotFound = errors.New("github oauth token not found")

type Token struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	Scope       string    `json:"scope"`
	CreatedAt   time.Time `json:"created_at"`
}

type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Save(token Token) error {
	if strings.TrimSpace(s.path) == "" {
		return fmt.Errorf("missing token store path")
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now().UTC()
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal github oauth token: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create token store directory: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("write github oauth token store: %w", err)
	}
	return nil
}

func (s *Store) Load() (Token, error) {
	if strings.TrimSpace(s.path) == "" {
		return Token{}, fmt.Errorf("missing token store path")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Token{}, ErrTokenNotFound
		}
		return Token{}, fmt.Errorf("read github oauth token store: %w", err)
	}
	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return Token{}, fmt.Errorf("parse github oauth token store: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return Token{}, ErrTokenNotFound
	}
	return token, nil
}

func (s *Store) Delete() error {
	if strings.TrimSpace(s.path) == "" {
		return fmt.Errorf("missing token store path")
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete github oauth token store: %w", err)
	}
	return nil
}
