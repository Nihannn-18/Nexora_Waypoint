// Package media abstracts where proof-of-delivery and shortfall photos live.
//
// The domain never calls an AWS SDK directly. A single Storage interface has
// two implementations, chosen by configuration:
//
//	local filesystem  (MEDIA_STORAGE=local) — the Docker Compose fallback
//	private S3 bucket (MEDIA_STORAGE=s3)    — the hosted deployment
//
// Object keys are always generated server-side from the domain context (never
// taken from the client), so a client cannot target an arbitrary object. The
// HTTP layer authorises the user and the target record before any key is
// minted or any bytes are read.
package media

import (
	"context"
	"fmt"
	"io"
	"strings"

	"waypoint.lk/api/internal/config"
)

// Storage is the media backend contract. Implementations must be safe for
// concurrent use.
type Storage interface {
	// Put stores the object at key and returns the canonical file reference to
	// persist on the domain record. key must be server-generated.
	Put(ctx context.Context, key string, r io.Reader, contentType string) error

	// Get opens the object at key for reading, returning a reader and the
	// stored content type. The caller closes the reader.
	Get(ctx context.Context, key string) (io.ReadCloser, string, error)

	// Delete removes the object at key. Removing a missing object is not an
	// error, so a retried cleanup is safe.
	Delete(ctx context.Context, key string) error

	// PresignPut returns a URL the client may PUT bytes to, and the headers it
	// must send. For the local backend the URL is the API's own upload route;
	// for S3 it is a presigned bucket URL. expiresIn is in seconds.
	PresignPut(ctx context.Context, key, contentType string, expiresIn int64) (url string, headers map[string]string, err error)

	// PresignGet returns a URL the client may GET the object from. For the
	// local backend it is the API's own download route; for S3 it is a
	// presigned bucket URL.
	PresignGet(ctx context.Context, key string, expiresIn int64) (url string, err error)
}

// Purpose scopes a media object to the workflow that produced it. It is part
// of the key, so a key is self-describing and can never be repurposed.
type Purpose string

const (
	// PurposeShortfall is a loader's missing/damaged photograph on an order line.
	PurposeShortfall Purpose = "shortfall"
	// PurposePOD is a driver's proof-of-delivery photograph on a leg.
	PurposePOD Purpose = "pod"
)

// Valid reports whether p is a recognised purpose.
func (p Purpose) Valid() bool {
	return p == PurposeShortfall || p == PurposePOD
}

// KeyPattern is the server-side shape of every object key:
//
//	<purpose>/<ownerID>/<uuid>
//
// ownerID is the order-item id for a shortfall and the leg id for a POD, so the
// key is traceable to its business context without trusting the client. KeyFor
// builds the canonical key for a purpose/owner/new object id.
func KeyFor(p Purpose, ownerID, id string) (string, error) {
	if !p.Valid() {
		return "", fmt.Errorf("unknown media purpose %q", p)
	}
	if ownerID == "" {
		return "", fmt.Errorf("media owner id is required")
	}
	if id == "" {
		return "", fmt.Errorf("media object id is required")
	}
	return fmt.Sprintf("%s/%s/%s", p, ownerID, id), nil
}

// ParseKey splits a server-generated key back into its purpose and owner. The
// authorizer uses it to recover an existing object's business context without
// trusting anything beyond the key the server issued.
func ParseKey(key string) (Purpose, string, error) {
	if _, err := safeKey(key); err != nil {
		return "", "", err
	}
	parts := strings.Split(key, "/")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("malformed media key %q", key)
	}
	p := Purpose(parts[0])
	if !p.Valid() {
		return "", "", fmt.Errorf("unknown media purpose %q", parts[0])
	}
	if parts[1] == "" || parts[2] == "" {
		return "", "", fmt.Errorf("malformed media key %q", key)
	}
	return p, parts[1], nil
}

// NewStorage selects a backend from configuration.
func NewStorage(ctx context.Context, cfg config.Config) (Storage, error) {
	switch cfg.MediaStorage {
	case config.StorageLocal:
		return NewLocalStorage(cfg.MediaRoot)
	case config.StorageS3:
		return NewS3Storage(ctx, cfg.S3Bucket, cfg.AWSRegion)
	default:
		return nil, fmt.Errorf("unknown media storage %q", cfg.MediaStorage)
	}
}

// safeKey rejects keys that could escape the storage root. Keys are always
// server-generated, but this is cheap defence in depth against a future bug or
// a hand-crafted key reaching the backend.
func safeKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("empty media key")
	}
	if strings.HasPrefix(key, "/") || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid media key %q", key)
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("invalid media key %q", key)
		}
	}
	return key, nil
}
