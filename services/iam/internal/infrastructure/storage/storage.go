// Package storage provides file storage implementations using MinIO S3.
package storage

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/zerolog/log"
)

// Service defines the interface for file storage operations.
type Service interface {
	// UploadProfilePicture uploads a profile picture and returns the object URL.
	UploadProfilePicture(ctx context.Context, userID string, filename string, data io.Reader, size int64, contentType string) (string, error)
	// UploadCMSImage uploads a CMS image and returns the object URL.
	UploadCMSImage(ctx context.Context, folder string, filename string, data io.Reader, size int64, contentType string) (string, error)
	// DeleteObject deletes a file by its object key.
	DeleteObject(ctx context.Context, objectKey string) error
	// ExtractObjectKey extracts the object key from a full URL.
	ExtractObjectKey(fullURL string) string
}

// Config holds MinIO/S3 configuration.
type Config struct {
	Endpoint           string `mapstructure:"endpoint"`
	AccessKey          string `mapstructure:"access_key"`
	SecretKey          string `mapstructure:"secret_key"`
	Bucket             string `mapstructure:"bucket"`
	BasePath           string `mapstructure:"base_path"` // Service prefix inside shared bucket (e.g., "iam")
	UseSSL             bool   `mapstructure:"use_ssl"`
	InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"` // Skip TLS verification (for self-signed certs)
	Region             string `mapstructure:"region"`
	PublicURL          string `mapstructure:"public_url"` // External URL prefix
}

// MinIOService implements Service using MinIO S3 client.
type MinIOService struct {
	client    *minio.Client
	bucket    string
	basePath  string
	publicURL string
}

// NewMinIOService creates a new MinIO storage service.
// It initializes the client and ensures the bucket exists.
func NewMinIOService(cfg Config) (*MinIOService, error) {
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	}

	// Support self-signed TLS certificates.
	if cfg.UseSSL && cfg.InsecureSkipVerify {
		opts.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // user explicitly opted-in for self-signed certs
			},
		}
	}

	client, err := minio.New(cfg.Endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	svc := &MinIOService{
		client:    client,
		bucket:    cfg.Bucket,
		basePath:  strings.TrimRight(cfg.BasePath, "/"),
		publicURL: cfg.PublicURL,
	}

	// Ensure bucket exists.
	if err := svc.ensureBucket(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ensure bucket: %w", err)
	}

	log.Info().
		Str("endpoint", cfg.Endpoint).
		Str("bucket", cfg.Bucket).
		Bool("ssl", cfg.UseSSL).
		Msg("MinIO storage service initialized")

	return svc, nil
}

// ensureBucket creates the bucket if it doesn't exist and sets a public read policy.
func (s *MinIOService) ensureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if exists {
		return nil
	}

	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	// Set bucket policy to allow public read for avatars.
	policy := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Principal": {"AWS": ["*"]},
			"Action": ["s3:GetObject"],
			"Resource": ["arn:aws:s3:::%s/*"]
		}]
	}`, s.bucket)

	if err := s.client.SetBucketPolicy(ctx, s.bucket, policy); err != nil {
		log.Warn().Err(err).Str("bucket", s.bucket).Msg("failed to set public read policy on bucket")
	}

	log.Info().Str("bucket", s.bucket).Msg("bucket created with public read policy")
	return nil
}

// UploadProfilePicture uploads a profile picture to MinIO.
// Files are stored as: {basePath}/avatars/{userID}/{uuid}_{filename}
// Example with basePath="iam": iam/avatars/user-id/uuid.jpg
func (s *MinIOService) UploadProfilePicture(ctx context.Context, userID string, filename string, data io.Reader, size int64, contentType string) (string, error) {
	// Sanitize filename (keep only the extension).
	ext := path.Ext(filename)
	if ext == "" {
		ext = ".jpg"
	}
	objectName := fmt.Sprintf("%s/avatars/%s/%s%s", s.basePath, userID, uuid.New().String(), ext)

	_, err := s.client.PutObject(ctx, s.bucket, objectName, data, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload profile picture: %w", err)
	}

	// Build the public URL for the uploaded object.
	var objectURL string
	if s.publicURL != "" {
		objectURL = fmt.Sprintf("%s/%s/%s", strings.TrimRight(s.publicURL, "/"), s.bucket, objectName)
	} else {
		objectURL = fmt.Sprintf("%s/%s/%s", s.client.EndpointURL().String(), s.bucket, objectName)
	}

	log.Debug().
		Str("user_id", userID).
		Str("object", objectName).
		Str("url", objectURL).
		Msg("profile picture uploaded")

	return objectURL, nil
}

// UploadCMSImage uploads a CMS image to MinIO.
// Files are stored as: {basePath}/cms/{folder}/{uuid}_{filename}
// Example with basePath="iam": iam/cms/sections/uuid.jpg
func (s *MinIOService) UploadCMSImage(ctx context.Context, folder string, filename string, data io.Reader, size int64, contentType string) (string, error) {
	ext := path.Ext(filename)
	if ext == "" {
		ext = ".jpg"
	}
	objectName := fmt.Sprintf("%s/cms/%s/%s%s", s.basePath, folder, uuid.New().String(), ext)

	_, err := s.client.PutObject(ctx, s.bucket, objectName, data, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload CMS image: %w", err)
	}

	var objectURL string
	if s.publicURL != "" {
		objectURL = fmt.Sprintf("%s/%s/%s", strings.TrimRight(s.publicURL, "/"), s.bucket, objectName)
	} else {
		objectURL = fmt.Sprintf("%s/%s/%s", s.client.EndpointURL().String(), s.bucket, objectName)
	}

	log.Debug().
		Str("folder", folder).
		Str("object", objectName).
		Str("url", objectURL).
		Msg("CMS image uploaded")

	return objectURL, nil
}

// DeleteObject deletes a file from MinIO by its object key.
func (s *MinIOService) DeleteObject(ctx context.Context, objectKey string) error {
	if objectKey == "" {
		return nil
	}

	err := s.client.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object %s: %w", objectKey, err)
	}

	log.Debug().Str("object", objectKey).Msg("object deleted from storage")
	return nil
}

// ExtractObjectKey extracts the object key from a full URL.
// For example, given "https://example.com/iam-avatars/avatars/user-id/file.jpg",
// it returns "avatars/user-id/file.jpg".
func (s *MinIOService) ExtractObjectKey(fullURL string) string {
	if fullURL == "" {
		return ""
	}

	// Look for the bucket name in the URL and extract everything after it.
	bucketPrefix := "/" + s.bucket + "/"
	idx := strings.Index(fullURL, bucketPrefix)
	if idx >= 0 {
		return fullURL[idx+len(bucketPrefix):]
	}

	// Fallback: if URL contains basePath prefix, extract from there.
	if s.basePath != "" {
		pathPrefix := s.basePath + "/"
		pathIdx := strings.Index(fullURL, pathPrefix)
		if pathIdx >= 0 {
			return fullURL[pathIdx:]
		}
	}

	return fullURL
}
