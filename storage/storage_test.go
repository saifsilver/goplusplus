package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/saifsilver/goplusplus/storage"
)

func TestLocalStorageProviderAndS3(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gpp_storage_*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	// Local Storage Provider
	local := storage.NewLocalStorageProvider(tempDir)
	url, err := local.Upload(ctx, "avatars/user.jpg", []byte("fake_image_data"), "image/jpeg")
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if url != "/uploads/avatars/user.jpg" {
		t.Errorf("unexpected local URL: %s", url)
	}

	preURL, _ := local.GetPresignedURL(ctx, "avatars/user.jpg", 10*time.Minute)
	if preURL != "/uploads/avatars/user.jpg" {
		t.Errorf("unexpected presigned URL: %s", preURL)
	}

	if _, err := os.Stat(filepath.Join(tempDir, "avatars", "user.jpg")); os.IsNotExist(err) {
		t.Errorf("expected uploaded file to exist on disk")
	}

	// S3 Storage Provider
	s3 := storage.NewS3Client(storage.S3Config{
		Bucket: "mybucket",
		Region: "us-east-1",
	})
	s3URL, _ := s3.Upload(ctx, "docs/file.pdf", []byte("pdf_data"), "application/pdf")
	if s3URL != "https://mybucket.s3.us-east-1.amazonaws.com/docs/file.pdf" {
		t.Errorf("unexpected S3 URL: %s", s3URL)
	}

	cdnURL := storage.GenerateCloudFrontURL("cdn.example.com", "docs/file.pdf")
	if cdnURL != "https://cdn.example.com/docs/file.pdf" {
		t.Errorf("unexpected CloudFront URL: %s", cdnURL)
	}
}

func TestLocalStorageRejectsPathsOutsideRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "uploads")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("mkdir storage root: %v", err)
	}
	provider := storage.NewLocalStorageProvider(root)

	if _, err := provider.Upload(context.Background(), "../escape.txt", []byte("secret"), "text/plain"); err == nil {
		t.Fatal("expected traversal upload to fail")
	}
	if _, err := os.Stat(filepath.Join(parent, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("escape file exists or stat failed unexpectedly: %v", err)
	}

	outDir := filepath.Join(parent, "outside")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatalf("mkdir outside directory: %v", err)
	}
	if err := os.Symlink(outDir, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := provider.Upload(context.Background(), "link/escape.txt", []byte("secret"), "text/plain"); err == nil {
		t.Fatal("expected symlink escape upload to fail")
	}
}
