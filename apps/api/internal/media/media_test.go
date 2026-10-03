package media

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyFor(t *testing.T) {
	tests := []struct {
		name    string
		purpose Purpose
		owner   string
		id      string
		want    string
		wantErr bool
	}{
		{name: "shortfall key", purpose: PurposeShortfall, owner: "R001", id: "abc", want: "shortfall/R001/abc"},
		{name: "pod key", purpose: PurposePOD, owner: "LEG1", id: "xyz", want: "pod/LEG1/xyz"},
		{name: "unknown purpose", purpose: "other", owner: "R1", id: "a", wantErr: true},
		{name: "missing owner", purpose: PurposePOD, owner: "", id: "a", wantErr: true},
		{name: "missing id", purpose: PurposePOD, owner: "L1", id: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := KeyFor(tt.purpose, tt.owner, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("KeyFor = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSafeKeyRejectsTraversal(t *testing.T) {
	bad := []string{"", "  ", "/etc/passwd", "../../secret", "pod/../../x", "pod//x", "pod/./x"}
	for _, k := range bad {
		if _, err := safeKey(k); err == nil {
			t.Errorf("safeKey(%q) should have failed", k)
		}
	}
	good := []string{"pod/LEG1/abc", "shortfall/R001/xyz"}
	for _, k := range good {
		if _, err := safeKey(k); err != nil {
			t.Errorf("safeKey(%q) unexpectedly failed: %v", k, err)
		}
	}
}

func TestLocalStorageRoundTrip(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalStorage(root)
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	ctx := context.Background()
	key, _ := KeyFor(PurposePOD, "LEG1", "obj1")
	body := []byte("fake-jpeg-bytes")

	if err := store.Put(ctx, key, bytes.NewReader(body), "image/jpeg"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, ct, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	// Close before Delete: Windows refuses to remove a file that is still open.
	_ = rc.Close()
	if !bytes.Equal(got, body) {
		t.Fatalf("Get returned %q, want %q", got, body)
	}
	if ct != "image/jpeg" {
		t.Fatalf("content type = %q, want image/jpeg", ct)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := store.Get(ctx, key); err == nil {
		t.Fatal("expected Get to fail after Delete")
	}
}

func TestLocalStorageRejectsTraversalKey(t *testing.T) {
	store, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}
	if err := store.Put(context.Background(), "../../escape", strings.NewReader("x"), "text/plain"); err == nil {
		t.Fatal("expected Put to reject a traversal key")
	}
}

func TestLocalStoragePresignPointsAtAPI(t *testing.T) {
	store, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}
	key, _ := KeyFor(PurposeShortfall, "R001", "obj1")

	putURL, _, err := store.PresignPut(context.Background(), key, "image/jpeg", 300)
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	if putURL != "/api/v1/media/"+key {
		t.Fatalf("PresignPut URL = %q", putURL)
	}

	getURL, err := store.PresignGet(context.Background(), key, 300)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	if getURL != "/api/v1/media/"+key {
		t.Fatalf("PresignGet URL = %q", getURL)
	}
}

func TestNewLocalStorageCreatesRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "media")
	if _, err := NewLocalStorage(root); err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("media root was not created: %v", err)
	}
}
