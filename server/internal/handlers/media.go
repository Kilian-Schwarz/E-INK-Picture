package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"e-ink-picture/server/internal/services"
)

type MediaHandler struct {
	svc *services.ImageService
}

func NewMediaHandler(svc *services.ImageService) *MediaHandler {
	return &MediaHandler{svc: svc}
}

func (h *MediaHandler) ListImages(w http.ResponseWriter, r *http.Request) {
	files, err := h.svc.ListImages()
	if err != nil {
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, files)
}

func (h *MediaHandler) GetImage(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if filename == "" {
		jsonError(w, "Missing filename", http.StatusBadRequest)
		return
	}
	path, err := h.svc.GetImagePath(filename)
	if err != nil {
		if errors.Is(err, services.ErrFileNotFound) || errors.Is(err, services.ErrInvalidFilename) {
			jsonError(w, "File not found!", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".bmp":
		w.Header().Set("Content-Type", "image/bmp")
	default:
		w.Header().Set("Content-Type", "image/png")
	}
	http.ServeFile(w, r, path)
}

func (h *MediaHandler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if body.Filename == "" {
		jsonError(w, "No filename provided", http.StatusBadRequest)
		return
	}
	if err := h.svc.DeleteImage(body.Filename); err != nil {
		if errors.Is(err, services.ErrFileNotFound) {
			jsonError(w, "File not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"message": "Image deleted"})
}

func (h *MediaHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "No file part", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename := header.Filename
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".png", ".jpg", ".jpeg":
		if err := h.svc.SaveImage(filename, file); err != nil {
			if errors.Is(err, services.ErrFileTooLarge) {
				jsonError(w, "File too large", http.StatusBadRequest)
				return
			}
			if errors.Is(err, services.ErrInvalidFilename) || errors.Is(err, services.ErrInvalidFileType) {
				jsonError(w, "Invalid file type!", http.StatusBadRequest)
				return
			}
			jsonError(w, "Image processing failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		baseName := strings.TrimSuffix(filepath.Base(filename), ext)
		pngName := baseName + ".png"
		jsonResponse(w, http.StatusOK, map[string]string{
			"message":   "File uploaded successfully!",
			"file_path": pngName,
		})

	case ".ttf", ".otf":
		if err := h.svc.SaveFont(filename, file); err != nil {
			if errors.Is(err, services.ErrFileTooLarge) {
				jsonError(w, "File too large", http.StatusBadRequest)
				return
			}
			jsonError(w, "Font upload failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{
			"message":   "Font uploaded successfully!",
			"font_path": filepath.Base(filename),
		})

	default:
		jsonError(w, "Invalid file type!", http.StatusBadRequest)
	}
}

func (h *MediaHandler) ListFonts(w http.ResponseWriter, r *http.Request) {
	files, err := h.svc.ListFonts()
	if err != nil {
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, files)
}

func (h *MediaHandler) GetFont(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if filename == "" {
		jsonError(w, "Missing filename", http.StatusBadRequest)
		return
	}
	path, err := h.svc.GetFontPath(filename)
	if err != nil {
		if errors.Is(err, services.ErrFileNotFound) || errors.Is(err, services.ErrInvalidFilename) {
			jsonError(w, "Font not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, path)
}
