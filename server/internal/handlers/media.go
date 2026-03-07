package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"e-ink-picture/server/internal/services"
)

type MediaHandler struct {
	svc *services.ImageService
}

func NewMediaHandler(svc *services.ImageService) *MediaHandler {
	return &MediaHandler{svc: svc}
}

// --- Legacy endpoints (keep existing frontend working) ---

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

// --- New Media Library API endpoints ---

// APIListImages returns paginated, sorted, filtered image metadata.
// GET /api/media/images?sort=date_desc&search=foo&page=1&limit=20
func (h *MediaHandler) APIListImages(w http.ResponseWriter, r *http.Request) {
	sortBy := r.URL.Query().Get("sort")
	search := r.URL.Query().Get("search")
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	limit := 20
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	resp, err := h.svc.ListImagesWithMeta(sortBy, search, page, limit)
	if err != nil {
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, resp)
}

// APIListFonts returns font metadata with optional search.
// GET /api/media/fonts?search=foo
func (h *MediaHandler) APIListFonts(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")

	fonts, err := h.svc.ListFontsWithMeta(search)
	if err != nil {
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"fonts": fonts,
	})
}

// APIGetThumb serves a thumbnail image.
// GET /api/media/images/thumb/{filename}
func (h *MediaHandler) APIGetThumb(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if filename == "" {
		jsonError(w, "Missing filename", http.StatusBadRequest)
		return
	}
	path, err := h.svc.GetThumbPath(filename)
	if err != nil {
		if errors.Is(err, services.ErrFileNotFound) || errors.Is(err, services.ErrInvalidFilename) {
			jsonError(w, "Thumbnail not found", http.StatusNotFound)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, path)
}

// APIUploadImage uploads an image with thumbnail generation and metadata.
// POST /api/media/images/upload
func (h *MediaHandler) APIUploadImage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "No file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename := header.Filename
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		jsonError(w, "Invalid file type. Allowed: png, jpg, jpeg", http.StatusBadRequest)
		return
	}

	savedName, meta, err := h.svc.SaveImageWithMeta(filename, file)
	if err != nil {
		if errors.Is(err, services.ErrFileTooLarge) {
			jsonError(w, "File too large (max 10MB)", http.StatusBadRequest)
			return
		}
		if errors.Is(err, services.ErrInvalidFilename) {
			jsonError(w, "Invalid filename", http.StatusBadRequest)
			return
		}
		if errors.Is(err, services.ErrInvalidFileType) {
			jsonError(w, "Invalid image file", http.StatusBadRequest)
			return
		}
		jsonError(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"message":  "Image uploaded successfully",
		"filename": savedName,
		"meta":     meta,
	})
}

// APIUploadFont uploads a font file with metadata.
// POST /api/media/fonts/upload
func (h *MediaHandler) APIUploadFont(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 5*1024*1024)

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "No file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename := header.Filename
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".ttf" && ext != ".otf" {
		jsonError(w, "Invalid file type. Allowed: ttf, otf", http.StatusBadRequest)
		return
	}

	savedName, _, err := h.svc.SaveFontWithMeta(filename, file)
	if err != nil {
		if errors.Is(err, services.ErrFileTooLarge) {
			jsonError(w, "File too large (max 5MB)", http.StatusBadRequest)
			return
		}
		if errors.Is(err, services.ErrInvalidFilename) {
			jsonError(w, "Invalid filename", http.StatusBadRequest)
			return
		}
		if errors.Is(err, services.ErrInvalidFileType) {
			jsonError(w, "Invalid font file", http.StatusBadRequest)
			return
		}
		jsonError(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{
		"message":  "Font uploaded successfully",
		"filename": savedName,
	})
}

// APIDeleteImage deletes an image, its thumbnail, and metadata.
// DELETE /api/media/images/{filename}
func (h *MediaHandler) APIDeleteImage(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if filename == "" {
		jsonError(w, "Missing filename", http.StatusBadRequest)
		return
	}

	if err := h.svc.DeleteImageWithMeta(filename); err != nil {
		if errors.Is(err, services.ErrFileNotFound) {
			jsonError(w, "Image not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, services.ErrInvalidFilename) {
			jsonError(w, "Invalid filename", http.StatusBadRequest)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"message": "Image deleted"})
}

// APIDeleteFont deletes a font file and its metadata.
// DELETE /api/media/fonts/{filename}
func (h *MediaHandler) APIDeleteFont(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if filename == "" {
		jsonError(w, "Missing filename", http.StatusBadRequest)
		return
	}

	if err := h.svc.DeleteFontWithMeta(filename); err != nil {
		if errors.Is(err, services.ErrFileNotFound) {
			jsonError(w, "Font not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, services.ErrInvalidFilename) {
			jsonError(w, "Invalid filename", http.StatusBadRequest)
			return
		}
		jsonError(w, "Server error", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"message": "Font deleted"})
}
