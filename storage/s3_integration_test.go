package storage_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/saifsilver/goplusplus/storage"
)

func TestS3UploadAndPresignIntegration(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "integration-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "integration-secret-key")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	payload := []byte("pdf-data")
	requestSeen := make(chan *http.Request, 1)
	bodySeen := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestSeen <- request.Clone(context.Background())
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read S3 request: %v", err)
		}
		bodySeen <- body
		writer.Header().Set("ETag", `"etag"`)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := storage.NewS3Client(context.Background(), storage.S3Config{
		Bucket: "integration-bucket", Region: "us-east-1", Prefix: "tenant-a",
		EndpointURL: server.URL, UsePathStyle: true, AllowInsecureHTTP: true, MaxUploadBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	objectURL, err := client.Upload(context.Background(), "docs/file.pdf", payload, "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	if objectURL != server.URL+"/integration-bucket/tenant-a/docs/file.pdf" {
		t.Fatalf("object URL = %q", objectURL)
	}
	request := <-requestSeen
	if request.Method != http.MethodPut || request.URL.Path != "/integration-bucket/tenant-a/docs/file.pdf" {
		t.Fatalf("S3 request = %s %s", request.Method, request.URL.Path)
	}
	if !strings.HasPrefix(request.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
		t.Fatalf("missing SigV4 authorization: %q", request.Header.Get("Authorization"))
	}
	if request.Header.Get("X-Amz-Server-Side-Encryption") != "AES256" {
		t.Fatalf("encryption = %q", request.Header.Get("X-Amz-Server-Side-Encryption"))
	}
	sum := sha256.Sum256(payload)
	if request.Header.Get("X-Amz-Checksum-Sha256") != base64.StdEncoding.EncodeToString(sum[:]) {
		t.Fatalf("checksum = %q", request.Header.Get("X-Amz-Checksum-Sha256"))
	}
	if body := <-bodySeen; string(body) != string(payload) {
		t.Fatalf("body = %q", body)
	}

	presigned, err := client.GetPresignedURL(context.Background(), "docs/file.pdf", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(presigned)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/integration-bucket/tenant-a/docs/file.pdf" || parsed.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("presigned URL = %q", presigned)
	}
}

func TestS3ValidationFailsBeforeNetwork(t *testing.T) {
	if _, err := storage.NewS3Client(context.Background(), storage.S3Config{}); err == nil {
		t.Fatal("empty S3 configuration was accepted")
	}
	if _, err := storage.NewS3Client(context.Background(), storage.S3Config{
		Bucket: "valid-bucket", Region: "us-east-1", PublicBaseURL: "http://cdn.example.com",
	}); err == nil {
		t.Fatal("insecure public S3 URL was accepted")
	}
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	client, err := storage.NewS3Client(context.Background(), storage.S3Config{
		Bucket: "valid-bucket", Region: "us-east-1", MaxUploadBytes: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Upload(context.Background(), "../escape", nil, "text/plain"); err == nil {
		t.Fatal("invalid S3 key was accepted")
	}
	if _, err := client.Upload(context.Background(), "object", []byte("large"), "text/plain"); err == nil {
		t.Fatal("oversized S3 object was accepted")
	}
	if _, err := client.GetPresignedURL(context.Background(), "object", 8*24*time.Hour); err == nil {
		t.Fatal("overlong S3 presign expiry was accepted")
	}
}
