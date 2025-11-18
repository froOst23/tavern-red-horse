package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"red-horse-tavern/internal/api"
	"red-horse-tavern/internal/storage"
	"red-horse-tavern/internal/utils"
)

var configPath string

func init() {
	flag.StringVar(&configPath, "config", "application.yaml", "Path to the config file")
	flag.Parse()
}

func main() {
	conf, err := utils.LoadConfig(configPath)
	if err != nil {
		panic("Cannot load app config: " + err.Error())
	}

	log := utils.SetupLogger(conf.App.LogLevel)
	slog.SetDefault(log)
	log.Info("Starting service...", "config", configPath)

	// PostgreSQL setup
	pg, err := storage.NewPostgresFromConfig(conf)
	if err != nil {
		log.Error("Failed to connect to Postgres", "error", err)
		os.Exit(1)
	}
	log.Info("Connected to Postgres")

	// Minio setup
	minioClient, err := storage.NewMinioFromConfig(conf)
	if err != nil {
		log.Error("Failed to connect to Minio", "error", err)
		os.Exit(1)
	}
	log.Info("Connected to Minio")

	// Create app
	app := api.NewApp(pg, minioClient.Client, conf, log)

	// Create router
	router := app.NewRouter()

	log.Info("Server is running on port " + conf.App.Port)
	err = http.ListenAndServe(":"+conf.App.Port, router)
	if err != nil {
		log.Error("Error starting service", "error", err)
		os.Exit(1)
	}
}
