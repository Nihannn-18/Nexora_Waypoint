package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Storage persists objects in a private S3 bucket. The bucket is never
// public; clients receive short-lived presigned URLs, and the API authorises
// the user and the record before minting one.
//
// Credentials come from the standard AWS SDK chain (an EC2 instance role in
// production, SSO locally). They are never read from the repository or passed
// as configuration values.
type S3Storage struct {
	bucket  string
	region  string
	client  *s3.Client
	presign *s3.PresignClient
}

// NewS3Storage loads the default AWS configuration and builds a client for the
// given bucket and region.
func NewS3Storage(ctx context.Context, bucket, region string) (*S3Storage, error) {
	if bucket == "" {
		return nil, errors.New("S3 bucket is required")
	}
	if region == "" {
		return nil, errors.New("AWS region is required")
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg)
	return &S3Storage{
		bucket:  bucket,
		region:  region,
		client:  client,
		presign: s3.NewPresignClient(client),
	}, nil
}

// Put uploads the object. The bucket has no public access policy, so the
// object is reachable only through a presigned URL.
func (s *S3Storage) Put(ctx context.Context, key string, r io.Reader, contentType string) error {
	clean, err := safeKey(key)
	if err != nil {
		return err
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(clean),
		Body:        r,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("s3 put %q: %w", clean, err)
	}
	return nil
}

// Get streams the object. Missing objects surface as os.ErrNotExist so the HTTP
// layer can map them to 404 like the local backend.
func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	clean, err := safeKey(key)
	if err != nil {
		return nil, "", err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(clean),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, "", fmt.Errorf("%w: %s", io.EOF, clean)
		}
		return nil, "", fmt.Errorf("s3 get %q: %w", clean, err)
	}
	ct := "application/octet-stream"
	if out.ContentType != nil {
		ct = *out.ContentType
	}
	return out.Body, ct, nil
}

// Delete removes the object. Deleting a missing key is a no-op in S3, so a
// retried cleanup is safe.
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	clean, err := safeKey(key)
	if err != nil {
		return err
	}
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(clean),
	}); err != nil {
		return fmt.Errorf("s3 delete %q: %w", clean, err)
	}
	return nil
}

// PresignPut returns a presigned URL the client PUTs bytes to. The content
// type is signed, so the client must send exactly this header.
func (s *S3Storage) PresignPut(ctx context.Context, key, contentType string, expiresIn int64) (string, map[string]string, error) {
	clean, err := safeKey(key)
	if err != nil {
		return "", nil, err
	}
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(clean),
		ContentType: aws.String(contentType),
	}, func(o *s3.PresignOptions) {
		o.Expires = time.Duration(expiresIn) * time.Second
	})
	if err != nil {
		return "", nil, fmt.Errorf("presign put %q: %w", clean, err)
	}

	headers := make(map[string]string, len(req.SignedHeader))
	for k, v := range req.SignedHeader {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	return req.URL, headers, nil
}

// PresignGet returns a short-lived presigned download URL.
func (s *S3Storage) PresignGet(ctx context.Context, key string, expiresIn int64) (string, error) {
	clean, err := safeKey(key)
	if err != nil {
		return "", err
	}
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(clean),
	}, func(o *s3.PresignOptions) {
		o.Expires = time.Duration(expiresIn) * time.Second
	})
	if err != nil {
		return "", fmt.Errorf("presign get %q: %w", clean, err)
	}
	return req.URL, nil
}
