package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	defaultS3MaxUploadBytes = 16 << 20
	defaultS3Timeout        = 30 * time.Second
	maxS3PresignExpiry      = 7 * 24 * time.Hour
)

var ErrS3NotConfigured = errors.New("storage: S3 provider is not configured")

type S3Config struct {
	Bucket            string
	Region            string
	Prefix            string
	EndpointURL       string
	PublicBaseURL     string
	KMSKeyID          string
	UsePathStyle      bool
	AllowInsecureHTTP bool
	MaxUploadBytes    int
	Timeout           time.Duration
	RetryAttempts     int
}

type s3ObjectAPI interface {
	PutObject(context.Context, *awss3.PutObjectInput, ...func(*awss3.Options)) (*awss3.PutObjectOutput, error)
}

// S3Client stores private objects using the AWS credential chain, SHA-256
// integrity checks, and server-side encryption.
type S3Client struct {
	cfg       S3Config
	client    s3ObjectAPI
	presigner *awss3.PresignClient
}

func NewS3Client(ctx context.Context, config S3Config) (*S3Client, error) {
	config, err := normalizeS3Config(config)
	if err != nil {
		return nil, err
	}
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(config.Region),
	}
	if config.RetryAttempts > 0 {
		loadOptions = append(loadOptions, awsconfig.WithRetryMaxAttempts(config.RetryAttempts))
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("storage: load AWS configuration: %w", err)
	}
	client := awss3.NewFromConfig(awsConfig, func(options *awss3.Options) {
		options.UsePathStyle = config.UsePathStyle
		if config.EndpointURL != "" {
			options.BaseEndpoint = aws.String(config.EndpointURL)
		}
	})
	return &S3Client{cfg: config, client: client, presigner: awss3.NewPresignClient(client)}, nil
}

func (client *S3Client) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if client == nil || client.client == nil {
		return "", ErrS3NotConfigured
	}
	fullKey, err := client.objectKey(key)
	if err != nil {
		return "", err
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType == "" {
		return "", errors.New("storage: invalid S3 content type")
	}
	if len(data) > client.cfg.MaxUploadBytes {
		return "", fmt.Errorf("storage: S3 object exceeds %d-byte upload limit", client.cfg.MaxUploadBytes)
	}
	checksum := sha256.Sum256(data)
	input := &awss3.PutObjectInput{
		Bucket: aws.String(client.cfg.Bucket), Key: aws.String(fullKey),
		Body: bytes.NewReader(data), ContentLength: aws.Int64(int64(len(data))),
		ContentType: aws.String(mediaType), ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
		ChecksumSHA256:       aws.String(base64.StdEncoding.EncodeToString(checksum[:])),
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	}
	if client.cfg.KMSKeyID != "" {
		input.ServerSideEncryption = types.ServerSideEncryptionAwsKms
		input.SSEKMSKeyId = aws.String(client.cfg.KMSKeyID)
		input.BucketKeyEnabled = aws.Bool(true)
	}
	operationCtx, cancel := context.WithTimeout(ctx, client.cfg.Timeout)
	defer cancel()
	if _, err := client.client.PutObject(operationCtx, input); err != nil {
		return "", fmt.Errorf("storage: upload S3 object: %w", err)
	}
	return client.objectURL(fullKey)
}

func (client *S3Client) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if client == nil || client.presigner == nil {
		return "", ErrS3NotConfigured
	}
	if expiry < time.Second || expiry > maxS3PresignExpiry {
		return "", errors.New("storage: S3 presign expiry must be between 1 second and 7 days")
	}
	fullKey, err := client.objectKey(key)
	if err != nil {
		return "", err
	}
	operationCtx, cancel := context.WithTimeout(ctx, client.cfg.Timeout)
	defer cancel()
	request, err := client.presigner.PresignGetObject(
		operationCtx,
		&awss3.GetObjectInput{Bucket: aws.String(client.cfg.Bucket), Key: aws.String(fullKey)},
		func(options *awss3.PresignOptions) { options.Expires = expiry },
	)
	if err != nil {
		return "", fmt.Errorf("storage: presign S3 object: %w", err)
	}
	return request.URL, nil
}

func normalizeS3Config(config S3Config) (S3Config, error) {
	if !validS3Bucket(config.Bucket) {
		return S3Config{}, errors.New("storage: invalid S3 bucket name")
	}
	if strings.TrimSpace(config.Region) == "" || strings.ContainsAny(config.Region, "\r\n\t ") {
		return S3Config{}, errors.New("storage: valid S3 region is required")
	}
	if config.Prefix != "" {
		prefix, err := cleanObjectKey(strings.TrimSuffix(config.Prefix, "/"))
		if err != nil {
			return S3Config{}, fmt.Errorf("storage: invalid S3 prefix: %w", err)
		}
		config.Prefix = prefix
	}
	if config.EndpointURL != "" {
		if err := validateS3BaseURL(config.EndpointURL, config.AllowInsecureHTTP); err != nil {
			return S3Config{}, err
		}
	}
	if config.PublicBaseURL != "" {
		if err := validateS3BaseURL(config.PublicBaseURL, false); err != nil {
			return S3Config{}, err
		}
	}
	if config.MaxUploadBytes <= 0 {
		config.MaxUploadBytes = defaultS3MaxUploadBytes
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultS3Timeout
	}
	return config, nil
}

func (client *S3Client) objectKey(key string) (string, error) {
	cleanKey, err := cleanObjectKey(key)
	if err != nil {
		return "", err
	}
	if client.cfg.Prefix != "" {
		cleanKey = client.cfg.Prefix + "/" + cleanKey
	}
	if len(cleanKey) > 1024 {
		return "", errors.New("storage: S3 object key exceeds 1024 bytes")
	}
	return cleanKey, nil
}

func (client *S3Client) objectURL(key string) (string, error) {
	if client.cfg.PublicBaseURL != "" {
		base, _ := url.Parse(client.cfg.PublicBaseURL)
		base.Path = path.Join(base.Path, key)
		return base.String(), nil
	}
	if client.cfg.EndpointURL != "" {
		base, _ := url.Parse(client.cfg.EndpointURL)
		base.Path = path.Join(base.Path, client.cfg.Bucket, key)
		return base.String(), nil
	}
	return (&url.URL{
		Scheme: "https", Host: client.cfg.Bucket + ".s3." + client.cfg.Region + ".amazonaws.com",
		Path: key,
	}).String(), nil
}

func validateS3BaseURL(value string, allowInsecureHTTP bool) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "https" && !(allowInsecureHTTP && parsed.Scheme == "http")) {
		return errors.New("storage: invalid S3 endpoint or public base URL")
	}
	return nil
}

func validS3Bucket(bucket string) bool {
	if len(bucket) < 3 || len(bucket) > 63 || net.ParseIP(bucket) != nil || strings.Contains(bucket, "..") {
		return false
	}
	for index, character := range bucket {
		valid := character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' || character == '.'
		if !valid || (character == '-' || character == '.') && (index == 0 || index == len(bucket)-1) {
			return false
		}
	}
	return !strings.Contains(bucket, ".-") && !strings.Contains(bucket, "-.")
}
