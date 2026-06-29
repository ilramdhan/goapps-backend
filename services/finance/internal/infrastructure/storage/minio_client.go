// Package storage provides MinIO/S3 storage operations for the finance service.
//
// Used by the worker to persist generated artifacts (Excel exports, etc.) to
// the shared `goapps-staging` bucket and by gRPC handlers to issue presigned
// download URLs back to the BFF.
package storage

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/zerolog/log"
)

// Config holds MinIO connection configuration.
type Config struct {
	Endpoint           string `mapstructure:"endpoint"`
	AccessKey          string `mapstructure:"access_key"`
	SecretKey          string `mapstructure:"secret_key"`
	Bucket             string `mapstructure:"bucket"`
	UseSSL             bool   `mapstructure:"use_ssl"`
	InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"`
	Region             string `mapstructure:"region"`
	PublicURL          string `mapstructure:"public_url"`
}

// Service is the storage interface used by the finance worker.
type Service interface {
	// PutObject uploads an object at the given key. contentType drives the stored Content-Type.
	PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	// GetObject downloads an object by key. The caller must close the returned ReadCloser.
	GetObject(ctx context.Context, key string) (io.ReadCloser, int64, error)
	// GetObjectStream returns a streaming reader for the object at key without
	// buffering the whole payload, letting callers stream-parse large files.
	// The caller must close the returned ReadCloser.
	GetObjectStream(ctx context.Context, key string) (io.ReadCloser, error)
	// PresignPutURL returns a presigned HTTP PUT URL valid for `expiry`, used by
	// the browser to upload a file directly to MinIO without it transiting the BFF.
	PresignPutURL(ctx context.Context, key string, expiry time.Duration) (string, error)
	// PresignedGetURL returns a signed download URL valid for `validity`.
	PresignedGetURL(ctx context.Context, key string, validity time.Duration, downloadName string) (string, error)
	// PresignedGetURLWithDisposition returns a signed URL with the explicit
	// Content-Disposition header (e.g. "inline" for in-browser preview).
	PresignedGetURLWithDisposition(ctx context.Context, key string, validity time.Duration, downloadName, disposition string) (string, error)
	// RemoveObject deletes an object. No-op if it doesn't exist.
	RemoveObject(ctx context.Context, key string) error
	// Bucket returns the configured bucket name.
	Bucket() string
}

// MinIOClient is the production implementation of Service. Two underlying
// minio.Client instances are kept: `upload` talks to the in-cluster endpoint
// (fast + private), while `presign` is built against the publicly-reachable
// endpoint so the AWS SigV4 signature is bound to the host the browser will
// actually call. This avoids "NoSuchKey/SignatureDoesNotMatch" errors caused
// by simply rewriting the host of an internal-signed URL post-hoc.
type MinIOClient struct {
	upload  *minio.Client
	presign *minio.Client // == upload when no public endpoint configured
	bucket  string
}

// NewMinIOClient builds a configured MinIO client; bucket is NOT created here
// (the operator/init container is expected to provision it).
func NewMinIOClient(cfg Config) (*MinIOClient, error) {
	uploadClient, err := buildClient(cfg.Endpoint, cfg.UseSSL, cfg)
	if err != nil {
		return nil, fmt.Errorf("create upload client: %w", err)
	}
	log.Info().Str("endpoint", cfg.Endpoint).Str("bucket", cfg.Bucket).Bool("ssl", cfg.UseSSL).Msg("MinIO upload client initialized")

	presignClient := uploadClient
	if cfg.PublicURL != "" {
		host, secure, perr := parsePublicEndpoint(cfg.PublicURL, cfg.UseSSL)
		if perr != nil {
			return nil, fmt.Errorf("parse public_url: %w", perr)
		}
		pc, perr := buildClient(host, secure, cfg)
		if perr != nil {
			return nil, fmt.Errorf("create presign client: %w", perr)
		}
		log.Info().Str("endpoint", host).Bool("ssl", secure).Msg("MinIO presign client initialized (public endpoint)")
		presignClient = pc
	}

	return &MinIOClient{upload: uploadClient, presign: presignClient, bucket: cfg.Bucket}, nil
}

// buildClient assembles a minio.Client with shared credentials/region but a
// caller-chosen endpoint and TLS mode. Self-signed certs are tolerated when
// the operator has opted in via InsecureSkipVerify.
func buildClient(endpoint string, secure bool, cfg Config) (*minio.Client, error) {
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: secure,
		Region: cfg.Region,
	}
	if secure && cfg.InsecureSkipVerify {
		opts.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // operator-opted self-signed certs
		}
	}
	return minio.New(endpoint, opts)
}

// parsePublicEndpoint converts a publicURL like "https://host:port" into the
// (host:port, secure) pair minio.New() expects. Falls back to the configured
// secure flag when the URL has no explicit scheme.
func parsePublicEndpoint(publicURL string, defaultSecure bool) (string, bool, error) {
	u, err := url.Parse(strings.TrimRight(publicURL, "/"))
	if err != nil {
		return "", false, err
	}
	host := u.Host
	if host == "" {
		// Bare "host:port" without scheme — url.Parse puts it in Path.
		host = u.Path
	}
	if host == "" {
		return "", false, fmt.Errorf("public_url has no host: %q", publicURL)
	}
	secure := defaultSecure
	switch strings.ToLower(u.Scheme) {
	case "https":
		secure = true
	case "http":
		secure = false
	}
	return host, secure, nil
}

// PutObject implements Service.
func (m *MinIOClient) PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	if _, err := m.upload.PutObject(ctx, m.bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}

// GetObject implements Service. Downloads the object at key from the upload (internal) endpoint.
// The caller is responsible for closing the returned ReadCloser.
func (m *MinIOClient) GetObject(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	obj, err := m.upload.GetObject(ctx, m.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("get object %s: %w", key, err)
	}
	stat, err := obj.Stat()
	if err != nil {
		if closeErr := obj.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Str("key", key).Msg("storage: close object after stat failure")
		}
		return nil, 0, fmt.Errorf("stat object %s: %w", key, err)
	}
	return obj, stat.Size, nil
}

// GetObjectStream implements Service. Returns the object at key as a streaming
// reader from the upload (internal) endpoint, avoiding a Stat round-trip so the
// worker can stream-parse without buffering the whole file. The caller is
// responsible for closing the returned ReadCloser.
func (m *MinIOClient) GetObjectStream(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := m.upload.GetObject(ctx, m.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object stream %s: %w", key, err)
	}
	return obj, nil
}

// PresignPutURL implements Service. Signed against the public endpoint so the
// browser-supplied Host header matches what was signed.
func (m *MinIOClient) PresignPutURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	u, err := m.presign.PresignedPutObject(ctx, m.bucket, key, expiry)
	if err != nil {
		return "", fmt.Errorf("presign put %s: %w", key, err)
	}
	return u.String(), nil
}

// PresignedGetURL implements Service. Signed against the public endpoint so
// the browser-supplied Host header matches what was signed.
func (m *MinIOClient) PresignedGetURL(ctx context.Context, key string, validity time.Duration, downloadName string) (string, error) {
	return m.PresignedGetURLWithDisposition(ctx, key, validity, downloadName, "attachment")
}

// PresignedGetURLWithDisposition is the explicit-disposition variant used by
// the attachments handler to switch between inline (in-browser preview) and
// attachment (force download) based on the file's MIME type.
func (m *MinIOClient) PresignedGetURLWithDisposition(ctx context.Context, key string, validity time.Duration, downloadName, disposition string) (string, error) {
	if disposition == "" {
		disposition = "attachment"
	}
	reqParams := url.Values{}
	if downloadName != "" {
		reqParams.Set("response-content-disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, sanitizeDownloadName(downloadName)))
	}
	u, err := m.presign.PresignedGetObject(ctx, m.bucket, key, validity, reqParams)
	if err != nil {
		return "", fmt.Errorf("presign get %s: %w", key, err)
	}
	return u.String(), nil
}

// RemoveObject implements Service.
func (m *MinIOClient) RemoveObject(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	if err := m.upload.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("remove object %s: %w", key, err)
	}
	return nil
}

// Bucket implements Service.
func (m *MinIOClient) Bucket() string { return m.bucket }

// sanitizeDownloadName strips characters that would break the
// Content-Disposition header without trying to be perfect.
func sanitizeDownloadName(s string) string {
	r := strings.NewReplacer(`"`, "", `\`, "", "\n", " ", "\r", " ")
	return r.Replace(s)
}
