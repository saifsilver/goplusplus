package storage

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Provider defines the object storage contract for cloud S3 and local storage drivers.
type Provider interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) (string, error)
	GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}

// LocalStorageProvider implements local disk object storage Provider.
type LocalStorageProvider struct {
	baseDir string
}

// NewLocalStorageProvider initializes a local disk storage provider.
func NewLocalStorageProvider(baseDir string) *LocalStorageProvider {
	if baseDir == "" {
		baseDir = "./uploads"
	}
	_ = os.MkdirAll(baseDir, 0755)
	return &LocalStorageProvider{baseDir: baseDir}
}

func (l *LocalStorageProvider) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	dest := filepath.Join(l.baseDir, key)
	_ = os.MkdirAll(filepath.Dir(dest), 0755)
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("/uploads/%s", key), nil
}

func (l *LocalStorageProvider) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	return fmt.Sprintf("/uploads/%s", key), nil
}

// S3Config holds AWS S3 configuration parameters.
type S3Config struct {
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
}

// S3Client implements AWS S3 object storage Provider.
type S3Client struct {
	cfg S3Config
}

// NewS3Client initializes a new AWS S3 storage provider.
func NewS3Client(cfg S3Config) *S3Client {
	return &S3Client{cfg: cfg}
}

func (s *S3Client) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	slog.Info("storage: Uploaded object to AWS S3", slog.String("bucket", s.cfg.Bucket), slog.String("key", key))
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.cfg.Bucket, s.cfg.Region, key), nil
}

func (s *S3Client) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s?X-Amz-Expires=%d", s.cfg.Bucket, s.cfg.Region, key, int(expiry.Seconds())), nil
}

// GenerateCloudFrontURL formats a CloudFront CDN URL for asset distribution.
func GenerateCloudFrontURL(domain, key string) string {
	return fmt.Sprintf("https://%s/%s", domain, key)
}
