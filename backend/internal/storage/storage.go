// Package storage reaches the S3-compatible object store that holds call recordings.
// Objects are private: browsers only ever get short-lived presigned links.
package storage

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"callcenter/internal/platform/config"
)

// Store is what the application needs from object storage.
type Store interface {
	// EnsureBucket creates the bucket when it does not exist yet.
	EnsureBucket(ctx context.Context) error
	// PresignGet returns a time-limited download URL for key. With a filename the browser is told
	// to save the file under that name; otherwise it may play it inline.
	PresignGet(ctx context.Context, key string, ttl time.Duration, filename string) (string, error)
	// Target describes where the recorder must upload, from the recorder's point of view.
	Target() Target
}

// Target is an upload destination for the recorder.
type Target struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	Secret    string
	Region    string
}

type minioStore struct {
	api       *minio.Client // API → store
	public    *minio.Client // only used to sign URLs for browsers
	bucket    string
	region    string
	accessKey string
	secretKey string
	egressURL string
}

func New(cfg config.StorageConfig) (Store, error) {
	opts := func() *minio.Options {
		return &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
			Secure: cfg.UseSSL,
			// A fixed region avoids a network round trip to look it up when signing.
			Region: cfg.Region,
		}
	}
	api, err := minio.New(cfg.Endpoint, opts())
	if err != nil {
		return nil, fmt.Errorf("storage client: %w", err)
	}
	public, err := minio.New(cfg.PublicEndpoint, opts())
	if err != nil {
		return nil, fmt.Errorf("storage public client: %w", err)
	}
	return &minioStore{
		api: api, public: public, bucket: cfg.Bucket, region: cfg.Region,
		accessKey: cfg.AccessKey, secretKey: cfg.SecretKey, egressURL: cfg.EgressEndpoint,
	}, nil
}

func (s *minioStore) EnsureBucket(ctx context.Context) error {
	exists, err := s.api.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check storage bucket: %w", err)
	}
	if exists {
		return nil
	}
	if err := s.api.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: s.region}); err != nil {
		return fmt.Errorf("create storage bucket: %w", err)
	}
	return nil
}

func (s *minioStore) PresignGet(ctx context.Context, key string, ttl time.Duration, filename string) (string, error) {
	params := url.Values{}
	if filename != "" {
		params.Set("response-content-disposition", fmt.Sprintf("attachment; filename=%q", filename))
	}
	u, err := s.public.PresignedGetObject(ctx, s.bucket, key, ttl, params)
	if err != nil {
		return "", fmt.Errorf("presign recording url: %w", err)
	}
	return u.String(), nil
}

func (s *minioStore) Target() Target {
	return Target{Endpoint: s.egressURL, Bucket: s.bucket, AccessKey: s.accessKey, Secret: s.secretKey, Region: s.region}
}
