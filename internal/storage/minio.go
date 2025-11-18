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
	slog.Info("Minio connection details",
		"endpoint", cfg.Minio.Endpoint,
		"access_key", cfg.Minio.AccessKey,
		"use_ssl", cfg.Minio.UseSSL)

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

	slog.Info("Successfully connected to MinIO")
	return &MinioClient{Client: client}, nil
}
