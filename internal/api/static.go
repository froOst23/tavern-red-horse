package api

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/minio/minio-go/v7"
)

func (a *App) ServeEventImage(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")

	a.Log.Info("ServeEventImage called", "filename", filename, "bucket", a.AppConfig.Minio.Bucket, "url", r.URL.String())

	// Проверяем существование объекта
	_, err := a.Minio.StatObject(r.Context(), a.AppConfig.Minio.Bucket, filename, minio.StatObjectOptions{})
	if err != nil {
		a.Log.Error("File not found in MinIO", "error", err, "filename", filename, "bucket", a.AppConfig.Minio.Bucket)
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	object, err := a.Minio.GetObject(r.Context(), a.AppConfig.Minio.Bucket, filename, minio.GetObjectOptions{})
	if err != nil {
		a.Log.Error("Failed to get object from MinIO", "error", err, "filename", filename)
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer func(object *minio.Object) {
		err := object.Close()
		if err != nil {
			a.Log.Error("Failed to close object", "error", err, "filename", filename)
			http.Error(w, "File not found", http.StatusNotFound)
		}
	}(object)

	// Получаем информацию о файле для Content-Type
	stat, err := object.Stat()
	if err != nil {
		a.Log.Error("Failed to get object stats", "error", err, "filename", filename)
		http.Error(w, "Failed to get file info", http.StatusInternalServerError)
		return
	}

	// Устанавливаем правильный Content-Type
	w.Header().Set("Content-Type", stat.ContentType)

	// Копируем объект из MinIO в ответ
	_, err = io.Copy(w, object)
	if err != nil {
		a.Log.Error("Failed to copy object to response", "error", err, "filename", filename)
		http.Error(w, "Failed to serve image", http.StatusInternalServerError)
		return
	}

	a.Log.Info("Successfully served event image", "filename", filename, "content_type", stat.ContentType, "size", stat.Size)
}
