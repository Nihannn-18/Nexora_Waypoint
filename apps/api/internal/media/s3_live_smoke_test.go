package media

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"
)

// TestS3LiveSmoke exercises the real S3 backend end to end when a bucket is
// supplied, and skips otherwise so the unit suite stays hermetic. It never runs
// in CI without WAYPOINT_S3_TEST_BUCKET.
func TestS3LiveSmoke(t *testing.T) {
	bucket := os.Getenv("WAYPOINT_S3_TEST_BUCKET")
	if bucket == "" {
		t.Skip("WAYPOINT_S3_TEST_BUCKET not set; skipping live S3 smoke")
	}
	region := os.Getenv("AWS_REGION")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, err := NewS3Storage(ctx, bucket, region)
	if err != nil {
		t.Fatalf("new s3 storage: %v", err)
	}
	key, err := KeyFor(PurposePOD, "smoke-leg", "smoke-object")
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	payload := []byte("waypoint-s3-smoke")
	if err := st.Put(ctx, key, bytes.NewReader(payload), "image/png"); err != nil {
		t.Fatalf("put: %v", err)
	}
	rc, ct, err := st.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != string(payload) {
		t.Fatalf("roundtrip mismatch: got %q ct=%s", got, ct)
	}
	url, err := st.PresignGet(ctx, key, 60)
	if err != nil || url == "" {
		t.Fatalf("presign get: %v", err)
	}
	if err := st.Delete(ctx, key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	t.Logf("S3 roundtrip OK via bucket %s (%s), presigned GET minted", bucket, region)
}
