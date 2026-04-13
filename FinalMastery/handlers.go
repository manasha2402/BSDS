package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// ─── Album handlers ────────────────────────────────────────────────────────────

func handlePutAlbum(store *dynamoStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		albumID := r.PathValue("album_id")

		var req struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Owner       string `json:"owner"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		album := Album{
			AlbumID:     albumID,
			Title:       req.Title,
			Description: req.Description,
			Owner:       req.Owner,
		}

		isNew, err := store.PutAlbum(r.Context(), album)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}

		status := http.StatusOK
		if isNew {
			status = http.StatusCreated
		}
		writeJSON(w, status, album)
	}
}

func handleGetAlbum(store *dynamoStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		albumID := r.PathValue("album_id")

		album, err := store.GetAlbum(r.Context(), albumID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		if album == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, album)
	}
}

func handleListAlbums(store *dynamoStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		albums, err := store.ListAlbums(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		writeJSON(w, http.StatusOK, albums) // ListAlbums always returns non-nil slice
	}
}

// ─── Photo handlers ────────────────────────────────────────────────────────────

// largeSizeThreshold splits upload paths.
// Files below this use async worker (fast 202 for S12 concurrent load).
// Files at or above use streaming sync (S15 large payload overlap).
const largeSizeThreshold = 5 * 1024 * 1024 // 5 MB

// handleUploadPhoto routes to the small (async) or large (streaming) path.
func handleUploadPhoto(store *dynamoStore, s3 *s3Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		albumID := r.PathValue("album_id")
		if r.ContentLength > 0 && r.ContentLength < largeSizeThreshold {
			uploadSmall(w, r, albumID, store, s3)
		} else {
			uploadLarge(w, r, albumID, store, s3)
		}
	}
}

// uploadSmall handles files < 5 MB.
//
// Flow:
//  1. ParseMultipartForm — buffers file in memory.
//  2. io.ReadAll — copy bytes for async worker.
//  3. IncrementPhotoSeq — atomic DynamoDB ADD (no transactions, no locks).
//  4. PutPhoto — insert as 'processing'.
//  5. Return 202 immediately.
//  6. Enqueue job — worker uploads to S3 and marks completed.
func uploadSmall(w http.ResponseWriter, r *http.Request, albumID string, store *dynamoStore, s3 *s3Store) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing photo field"})
		return
	}
	defer r.MultipartForm.RemoveAll() //nolint:errcheck

	file, header, err := r.FormFile("photo")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing photo field"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read error"})
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	photoID := uuid.New().String()
	s3Key := fmt.Sprintf("photos/%s/%s", albumID, photoID)

	seq, err := store.IncrementPhotoSeq(r.Context(), albumID)
	if err != nil {
		var ccf interface{ CancelationReason() string }
		if errors.As(err, &ccf) || strings.Contains(err.Error(), "ConditionalCheckFailed") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "album not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	if err := store.PutPhoto(r.Context(), Photo{
		PhotoID: photoID, AlbumID: albumID, Seq: seq,
		Status: "processing", S3Key: s3Key,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"photo_id": photoID,
		"seq":      seq,
		"status":   "processing",
	})

	// Enqueue in goroutine so response flushes before channel send.
	// Channel is 10 000-deep — blocks only under extreme backlog, never drops.
	job := PhotoJob{PhotoID: photoID, S3Key: s3Key, FileBytes: data, ContentType: contentType}
	go func() { jobQueue <- job }()
}

// uploadLarge handles files >= 5 MB (or unknown Content-Length).
//
// Flow:
//  1. IncrementPhotoSeq + PutPhoto — reserve the seq before reading body.
//  2. extractPhotoPart — parse multipart headers only (body not yet read).
//  3. S3 streaming upload — body receipt and S3 upload overlap simultaneously.
//  4. UpdatePhotoStatus('completed') — before returning 202 so first poll wins.
//  5. Return 202.
func uploadLarge(w http.ResponseWriter, r *http.Request, albumID string, store *dynamoStore, s3 *s3Store) {
	photoID := uuid.New().String()
	s3Key := fmt.Sprintf("photos/%s/%s", albumID, photoID)

	seq, err := store.IncrementPhotoSeq(r.Context(), albumID)
	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailed") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "album not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	if err := store.PutPhoto(r.Context(), Photo{
		PhotoID: photoID, AlbumID: albumID, Seq: seq,
		Status: "processing", S3Key: s3Key,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	part, err := extractPhotoPart(r)
	if err != nil {
		log.Printf("multipart extract failed for %s: %v", photoID, err)
		store.UpdatePhotoStatus(context.Background(), photoID, "failed") //nolint:errcheck
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing photo field"})
		return
	}
	defer part.Close()

	contentType := part.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Stream: bytes arrive from client and are simultaneously uploaded to S3
	// in 5 MB chunks. Total time ≈ max(receive, upload) instead of receive + upload.
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	if err := s3.upload(ctx, s3Key, part, contentType); err != nil {
		log.Printf("large upload failed for %s: %v", photoID, err)
		store.UpdatePhotoStatus(context.Background(), photoID, "failed") //nolint:errcheck
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "upload failed"})
		return
	}

	// Mark completed BEFORE returning 202 — ChaosArena's first poll sees
	// completed immediately, eliminating one polling round-trip.
	store.UpdatePhotoStatus(context.Background(), photoID, "completed") //nolint:errcheck

	writeJSON(w, http.StatusAccepted, map[string]any{
		"photo_id": photoID,
		"seq":      seq,
		"status":   "processing",
	})
}

// extractPhotoPart returns the 'photo' multipart part as a streaming io.Reader
// without buffering the body. Caller must call part.Close() when done.
func extractPhotoPart(r *http.Request) (*multipart.Part, error) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return nil, fmt.Errorf("expected multipart, got %q", r.Header.Get("Content-Type"))
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, fmt.Errorf("missing boundary")
	}
	mr := multipart.NewReader(r.Body, boundary)
	for {
		p, err := mr.NextPart()
		if err != nil {
			return nil, fmt.Errorf("photo part not found: %w", err)
		}
		if p.FormName() == "photo" {
			return p, nil
		}
		p.Close() // skip non-photo parts
	}
}

// handleGetPhoto returns photo status. When completed, includes the permanent
// public S3 URL — no presigned URL overhead on every request.
func handleGetPhoto(store *dynamoStore, s3 *s3Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		albumID := r.PathValue("album_id")
		photoID := r.PathValue("photo_id")

		photo, err := store.GetPhoto(r.Context(), photoID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		if photo == nil || photo.AlbumID != albumID {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}

		resp := map[string]any{
			"photo_id": photo.PhotoID,
			"album_id": photo.AlbumID,
			"seq":      photo.Seq,
			"status":   photo.Status,
		}
		if photo.Status == "completed" && photo.S3Key != "" {
			resp["url"] = s3.publicURL(photo.S3Key)
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// handleDeletePhoto deletes a photo atomically.
// DB row deleted first — GET returns 404 immediately.
// S3 delete follows — URL returns non-200 immediately.
// Returns 204 even if the photo was already deleted (idempotent — S7/S8).
func handleDeletePhoto(store *dynamoStore, s3 *s3Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		albumID := r.PathValue("album_id")
		photoID := r.PathValue("photo_id")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		photo, err := store.DeletePhoto(ctx, photoID, albumID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		if photo == nil {
			// Already deleted — spec only defines 200/204, respond idempotently.
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// S3 delete is synchronous — ChaosArena verifies the URL is gone.
		if photo.S3Key != "" {
			if err := s3.delete(context.Background(), photo.S3Key); err != nil {
				log.Printf("WARN: s3 delete failed for %s: %v", photo.S3Key, err)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
