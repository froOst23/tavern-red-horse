package storage

import (
	"context"
	"log/slog"
	"red-horse-tavern/internal/utils"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioClient struct {
	Client *minio.Client
}

func NewMinioFromConfig(cfg *utils.AppConfig) (*MinioClient, error) {
	client, err := minio.New(cfg.Minio.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.Minio.AccessKey, cfg.Minio.SecretKey, ""),
		Secure: cfg.Minio.UseSSL,
	})
	if err != nil {
		slog.Error("Failed to create MinIO client", "error", err)
		return nil, err
	}

	_, err = client.ListBuckets(context.Background())
	if err != nil {
		slog.Error("Failed to list buckets", "error", err)
		return nil, err
	}

	slog.Info("Connected to Minio: " + cfg.Minio.Endpoint)

	return &MinioClient{Client: client}, nil
}
