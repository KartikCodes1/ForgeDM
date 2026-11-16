package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/time/rate"
)

// Store persists the download list as JSON with atomic replace.
type Store struct {
	Path string
	mu   sync.Mutex
}

// Load reads saved downloads. A missing file is an empty list.
func (s *Store) Load() ([]*Download, error) {
	if s.Path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*Download
	if err := json.Unmarshal(b, &list); err != nil {
		// Keep the damaged file for inspection and start clean.
		_ = os.Rename(s.Path, s.Path+".damaged")
		return nil, nil
	}
	return list, nil
}

// Save writes the list atomically.
func (s *Store) Save(list []Download) error {
	if s.Path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return WriteFileAtomic(s.Path, b)
}

// WriteFileAtomic writes data to a temp file and renames it into place.
func WriteFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func waitN(ctx context.Context, l *rate.Limiter, n int) error {
	if l == nil || l.Limit() == rate.Inf {
		return nil
	}
	if b := l.Burst(); n > b {
		n = b
	}
	if n <= 0 {
		return nil
	}
	return l.WaitN(ctx, n)
}
