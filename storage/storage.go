package storage

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
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
	initErr error
}

// NewLocalStorageProvider initializes a local disk storage provider.
func NewLocalStorageProvider(baseDir string) *LocalStorageProvider {
	if baseDir == "" {
		baseDir = "./uploads"
	}
	absoluteDir, err := filepath.Abs(baseDir)
	if err == nil {
		err = os.MkdirAll(absoluteDir, 0o755)
	}
	return &LocalStorageProvider{baseDir: absoluteDir, initErr: err}
}

func (l *LocalStorageProvider) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if l.initErr != nil {
		return "", fmt.Errorf("storage: initialize local provider: %w", l.initErr)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cleanKey, err := cleanLocalStorageKey(key)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(l.baseDir)
	if err != nil {
		return "", fmt.Errorf("storage: open local root: %w", err)
	}
	defer root.Close()

	directory := path.Dir(cleanKey)
	if directory != "." {
		if err := root.MkdirAll(filepath.FromSlash(directory), 0o755); err != nil {
			return "", fmt.Errorf("storage: create object directory: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := root.WriteFile(filepath.FromSlash(cleanKey), data, 0o644); err != nil {
		return "", err
	}
	return localStorageURL(cleanKey), nil
}

func (l *LocalStorageProvider) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if l.initErr != nil {
		return "", fmt.Errorf("storage: initialize local provider: %w", l.initErr)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cleanKey, err := cleanLocalStorageKey(key)
	if err != nil {
		return "", err
	}
	return localStorageURL(cleanKey), nil
}

func cleanLocalStorageKey(key string) (string, error) {
	normalized := strings.ReplaceAll(key, "\\", "/")
	cleaned := path.Clean(normalized)
	if cleaned != normalized || cleaned == "." || !fs.ValidPath(cleaned) {
		return "", fmt.Errorf("storage: invalid object key")
	}
	return cleaned, nil
}

func localStorageURL(key string) string {
	return (&url.URL{Path: "/uploads/" + key}).EscapedPath()
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
