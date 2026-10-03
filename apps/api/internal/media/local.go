package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// metaSuffix names the sidecar file that records an object's content type.
const metaSuffix = ".meta"

// LocalStorage persists objects under a root directory on disk. It is the
// Docker Compose fallback and the default backend: no AWS credentials, and a
// named volume keeps uploads across restarts.
type LocalStorage struct {
	root string
}

// NewLocalStorage creates the root directory if it does not yet exist.
func NewLocalStorage(root string) (*LocalStorage, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve media root %q: %w", root, err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("create media root %q: %w", abs, err)
	}
	return &LocalStorage{root: abs}, nil
}

// Root reports the absolute media root, for logging.
func (l *LocalStorage) Root() string { return l.root }

func (l *LocalStorage) path(key string) (string, error) {
	clean, err := safeKey(key)
	if err != nil {
		return "", err
	}
	full := filepath.Join(l.root, filepath.FromSlash(clean))

	// Belt and braces: the joined path must still sit under the root.
	if !strings.HasPrefix(full, l.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("media key %q escapes the storage root", key)
	}
	return full, nil
}

// Put writes the object, creating parent directories as needed. The content
// type is stored in a sidecar ".meta" file so Get can return exactly what was
// uploaded rather than guessing from an extension.
func (l *LocalStorage) Put(_ context.Context, key string, r io.Reader, contentType string) error {
	full, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return fmt.Errorf("create media directory: %w", err)
	}

	// Write to a temp file then rename, so a partial upload is never visible
	// under the real key.
	tmp := full + ".part"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
	if err != nil {
		return fmt.Errorf("open media temp file: %w", err)
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write media: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close media file: %w", err)
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit media file: %w", err)
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := os.WriteFile(full+metaSuffix, []byte(contentType), 0o640); err != nil {
		return fmt.Errorf("write media metadata: %w", err)
	}
	return nil
}

// Get opens the object and returns its recorded content type. A missing object
// surfaces as os.ErrNotExist, which the HTTP layer maps to 404.
func (l *LocalStorage) Get(_ context.Context, key string) (io.ReadCloser, string, error) {
	full, err := l.path(key)
	if err != nil {
		return nil, "", err
	}
	f, err := os.Open(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", os.ErrNotExist
		}
		return nil, "", fmt.Errorf("open media: %w", err)
	}

	ct := "application/octet-stream"
	if raw, err := os.ReadFile(full + metaSuffix); err == nil {
		if s := strings.TrimSpace(string(raw)); s != "" {
			ct = s
		}
	}
	return f, ct, nil
}

// Delete removes the object and its metadata sidecar; a missing object is not
// an error.
func (l *LocalStorage) Delete(_ context.Context, key string) error {
	full, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete media: %w", err)
	}
	if err := os.Remove(full + metaSuffix); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete media metadata: %w", err)
	}
	return nil
}

// PresignPut for local storage is the API's own upload route: the HTTP layer
// authorises the request and calls Put. There is no bucket to sign.
func (l *LocalStorage) PresignPut(_ context.Context, key, _ string, _ int64) (string, map[string]string, error) {
	if _, err := safeKey(key); err != nil {
		return "", nil, err
	}
	return "/api/v1/media/" + key, nil, nil
}

// PresignGet for local storage is the API's own authorised download route.
func (l *LocalStorage) PresignGet(_ context.Context, key string, _ int64) (string, error) {
	if _, err := safeKey(key); err != nil {
		return "", err
	}
	return "/api/v1/media/" + key, nil
}
