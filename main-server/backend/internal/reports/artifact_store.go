package reports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

type ArtifactMeta struct {
	Key         string `json:"artifact_key"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

type ArtifactStore interface {
	Put(ctx context.Context, key string, raw []byte, contentType string) (ArtifactMeta, error)
	Get(ctx context.Context, key string) ([]byte, ArtifactMeta, error)
	Path(key string) (string, error)
	Exists(ctx context.Context, key string) bool
}

type LocalArtifactStore struct {
	fs   afero.Fs
	root string
}

func NewLocalArtifactStore(root string) *LocalArtifactStore {
	if strings.TrimSpace(root) == "" {
		root = filepath.Join("data", "report-assets")
	}
	return &LocalArtifactStore{fs: afero.NewOsFs(), root: root}
}

func (s *LocalArtifactStore) Put(_ context.Context, key string, raw []byte, contentType string) (ArtifactMeta, error) {
	path, err := s.safePath(key)
	if err != nil {
		return ArtifactMeta{}, err
	}
	if err := s.fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ArtifactMeta{}, err
	}
	if err := afero.WriteFile(s.fs, path, raw, 0o644); err != nil {
		return ArtifactMeta{}, err
	}
	return ArtifactMeta{
		Key:         key,
		ContentType: contentType,
		Size:        int64(len(raw)),
		SHA256:      sha256Hex(raw),
	}, nil
}

func (s *LocalArtifactStore) Get(_ context.Context, key string) ([]byte, ArtifactMeta, error) {
	path, err := s.safePath(key)
	if err != nil {
		return nil, ArtifactMeta{}, err
	}
	raw, err := afero.ReadFile(s.fs, path)
	if err != nil {
		return nil, ArtifactMeta{}, err
	}
	return raw, ArtifactMeta{
		Key:         key,
		ContentType: artifactContentType(key),
		Size:        int64(len(raw)),
		SHA256:      sha256Hex(raw),
	}, nil
}

func (s *LocalArtifactStore) Path(key string) (string, error) {
	return s.safePath(key)
}

func (s *LocalArtifactStore) Exists(_ context.Context, key string) bool {
	path, err := s.safePath(key)
	if err != nil {
		return false
	}
	info, err := s.fs.Stat(path)
	return err == nil && !info.IsDir()
}

func (s *LocalArtifactStore) safePath(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || filepath.IsAbs(key) || strings.Contains(key, "\\") {
		return "", fs.ErrInvalid
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fs.ErrInvalid
	}
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(root, clean))
	if err != nil {
		return "", err
	}
	if path != root && !strings.HasPrefix(path, root+string(filepath.Separator)) {
		return "", errors.New("artifact key escapes root")
	}
	return path, nil
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
