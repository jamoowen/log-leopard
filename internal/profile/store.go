package profile

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
)

var (
	projectIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
	profileIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
	ErrNotFound      = errors.New("profile not found")
	ErrValidation    = errors.New("profile validation failed")
)

type validationError struct{ message string }

func (e validationError) Error() string { return e.message }
func (e validationError) Unwrap() error { return ErrValidation }

func validation(message string) error { return validationError{message: message} }

func ValidationMessage(err error) (string, bool) {
	var validationErr validationError
	if !errors.As(err, &validationErr) {
		return "", false
	}
	return validationErr.message, true
}

type Profile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ProjectID string `json:"projectId"`
}

type Store struct {
	mu   sync.Mutex
	path string
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "LogLeopard", "connections.json"), nil
}

func NewStore(path string) *Store { return &Store{path: path} }

func Validate(p Profile) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len(p.Name) > 80 {
		return validation("name must contain 1 to 80 characters")
	}
	if !projectIDPattern.MatchString(p.ProjectID) {
		return validation("projectId must be a valid GCP project ID")
	}
	if p.ID != "" && !profileIDPattern.MatchString(p.ID) {
		return validation("id is invalid")
	}
	return nil
}

func (s *Store) List() ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

func (s *Store) Save(input Profile) (Profile, error) {
	input.Name = strings.TrimSpace(input.Name)
	if err := Validate(input); err != nil {
		return Profile{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles, err := s.read()
	if err != nil {
		return Profile{}, err
	}
	if input.ID == "" {
		id := sha256.Sum256([]byte(rand.Text()))
		input.ID = hex.EncodeToString(id[:16])
		profiles = append(profiles, input)
	} else {
		found := false
		for i := range profiles {
			if profiles[i].ID == input.ID {
				profiles[i] = input
				found = true
				break
			}
		}
		if !found {
			return Profile{}, ErrNotFound
		}
	}
	if err := s.write(profiles); err != nil {
		return Profile{}, err
	}
	return input, nil
}

func (s *Store) Delete(id string) error {
	if !profileIDPattern.MatchString(id) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles, err := s.read()
	if err != nil {
		return err
	}
	for i := range profiles {
		if profiles[i].ID == id {
			profiles = slices.Delete(profiles, i, i+1)
			return s.write(profiles)
		}
	}
	return ErrNotFound
}

func (s *Store) read() ([]Profile, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read profiles: %w", err)
	}
	var profiles []Profile
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&profiles); err != nil {
		return nil, fmt.Errorf("decode profiles: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("stored profiles contain trailing data")
	}
	seen := make(map[string]bool)
	for _, p := range profiles {
		if err := Validate(p); err != nil || seen[p.ID] {
			return nil, errors.New("stored profiles are invalid")
		}
		seen[p.ID] = true
	}
	return profiles, nil
}

func (s *Store) write(profiles []Profile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	b, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profiles: %w", err)
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".connections-*")
	if err != nil {
		return fmt.Errorf("create temporary profile file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure temporary profile file: %w", err)
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write profiles: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync profiles: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close profiles: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace profiles: %w", err)
	}
	dir, err := os.Open(filepath.Dir(s.path))
	if err != nil {
		return fmt.Errorf("open config directory: %w", err)
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return fmt.Errorf("sync config directory: %w", err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("close config directory: %w", err)
	}
	return nil
}
