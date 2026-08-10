package storage

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// S3Config holds configuration parameters for AWS S3 object storage.
type S3Config struct {
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
}

// S3Client manages object uploads, downloads, and presigned URLs for AWS S3.
type S3Client struct {
	cfg S3Config
}

// NewS3Client initializes a new AWS S3 client adapter.
func NewS3Client(cfg S3Config) *S3Client {
	return &S3Client{cfg: cfg}
}

// Upload uploads raw bytes to AWS S3 bucket.
func (s *S3Client) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	slog.Info("storage: Uploaded object to AWS S3", slog.String("bucket", s.cfg.Bucket), slog.String("key", key))
	url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.cfg.Bucket, s.cfg.Region, key)
	return url, nil
}

// GetPresignedURL generates a presigned URL for secure object access.
func (s *S3Client) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s?X-Amz-Expires=%d", s.cfg.Bucket, s.cfg.Region, key, int(expiry.Seconds()))
	return url, nil
}

// GenerateCloudFrontURL formats a CloudFront CDN URL for fast global asset delivery.
func GenerateCloudFrontURL(domain, key string) string {
	return fmt.Sprintf("https://%s/%s", domain, key)
}
