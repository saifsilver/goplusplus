package storage

import (
	"context"
	"fmt"
	"io/fs"
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

// Upload writes one object beneath the configured local storage root.
func (l *LocalStorageProvider) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if l.initErr != nil {
		return "", fmt.Errorf("storage: initialize local provider: %w", l.initErr)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cleanKey, err := cleanObjectKey(key)
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

// GetPresignedURL returns the local serving path; it does not create a signature.
func (l *LocalStorageProvider) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if l.initErr != nil {
		return "", fmt.Errorf("storage: initialize local provider: %w", l.initErr)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cleanKey, err := cleanObjectKey(key)
	if err != nil {
		return "", err
	}
	return localStorageURL(cleanKey), nil
}

func cleanObjectKey(key string) (string, error) {
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

// GenerateCloudFrontURL formats a CloudFront CDN URL for asset distribution.
func GenerateCloudFrontURL(domain, key string) string {
	return fmt.Sprintf("https://%s/%s", domain, key)
}
