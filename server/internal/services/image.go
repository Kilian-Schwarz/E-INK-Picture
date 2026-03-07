package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	// Register image decoders for JPEG and PNG.
	_ "image/jpeg"

	"e-ink-picture/server/internal/models"

	xdraw "golang.org/x/image/draw"
)

var (
	ErrFileNotFound    = errors.New("file not found")
	ErrInvalidFilename = errors.New("invalid filename")
	ErrInvalidFileType = errors.New("invalid file type")
	ErrFileTooLarge    = errors.New("file too large")
)

const (
	maxImageSize = 10 * 1024 * 1024 // 10 MB
	maxFontSize  = 5 * 1024 * 1024  // 5 MB
	thumbMaxDim  = 300              // max width/height for thumbnails
)

// validFilenameRe allows alphanumeric characters, hyphens, underscores, dots, and parentheses.
var validFilenameRe = regexp.MustCompile(`^[a-zA-Z0-9\-_.()\s]+$`)

type ImageService struct {
	imagesDir string
	fontsDir  string
	thumbsDir string
	dataDir   string
	metaPath  string
	mu        sync.RWMutex
}

func NewImageService(dataDir string) *ImageService {
	svc := &ImageService{
		imagesDir: dataDir + "/uploaded_images",
		fontsDir:  dataDir + "/fonts",
		thumbsDir: dataDir + "/uploaded_images/thumbs",
		dataDir:   dataDir,
		metaPath:  dataDir + "/media_meta.json",
	}
	// Ensure thumbs directory exists
	if err := os.MkdirAll(svc.thumbsDir, 0755); err != nil {
		slog.Error("failed to create thumbs directory", "error", err)
	}
	return svc
}

// sanitize validates and cleans a filename. Returns the sanitized name or an error.
func sanitize(name string) (string, error) {
	base := filepath.Base(name)
	if base == "." || base == ".." || strings.HasPrefix(base, ".") {
		return "", ErrInvalidFilename
	}
	if strings.Contains(name, "..") {
		return "", ErrInvalidFilename
	}
	if !validFilenameRe.MatchString(base) {
		return "", ErrInvalidFilename
	}
	return base, nil
}

// --- Metadata persistence ---

// LoadMeta reads media metadata from disk.
func (s *ImageService) LoadMeta() (models.MediaMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadMetaUnlocked()
}

func (s *ImageService) loadMetaUnlocked() (models.MediaMeta, error) {
	var meta models.MediaMeta
	data, err := os.ReadFile(s.metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return models.MediaMeta{
				Images: []models.ImageMeta{},
				Fonts:  []models.FontMeta{},
			}, nil
		}
		return meta, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		slog.Warn("corrupt media_meta.json, resetting", "error", err)
		return models.MediaMeta{
			Images: []models.ImageMeta{},
			Fonts:  []models.FontMeta{},
		}, nil
	}
	if meta.Images == nil {
		meta.Images = []models.ImageMeta{}
	}
	if meta.Fonts == nil {
		meta.Fonts = []models.FontMeta{}
	}
	return meta, nil
}

// SaveMeta writes media metadata to disk atomically.
func (s *ImageService) SaveMeta(meta models.MediaMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveMetaUnlocked(meta)
}

func (s *ImageService) saveMetaUnlocked(meta models.MediaMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := s.metaPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.metaPath)
}

// GetImageMeta returns metadata for a single image by filename.
func (s *ImageService) GetImageMeta(filename string) (models.ImageMeta, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	meta, err := s.loadMetaUnlocked()
	if err != nil {
		return models.ImageMeta{}, false
	}
	for _, img := range meta.Images {
		if img.Filename == filename {
			return img, true
		}
	}
	return models.ImageMeta{}, false
}

// AddImageMeta adds image metadata, replacing any existing entry with the same filename.
func (s *ImageService) AddImageMeta(imgMeta models.ImageMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, err := s.loadMetaUnlocked()
	if err != nil {
		return err
	}
	// Replace existing or append
	found := false
	for i, img := range meta.Images {
		if img.Filename == imgMeta.Filename {
			meta.Images[i] = imgMeta
			found = true
			break
		}
	}
	if !found {
		meta.Images = append(meta.Images, imgMeta)
	}
	return s.saveMetaUnlocked(meta)
}

// RemoveImageMeta removes image metadata by filename.
func (s *ImageService) RemoveImageMeta(filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, err := s.loadMetaUnlocked()
	if err != nil {
		return err
	}
	filtered := make([]models.ImageMeta, 0, len(meta.Images))
	for _, img := range meta.Images {
		if img.Filename != filename {
			filtered = append(filtered, img)
		}
	}
	meta.Images = filtered
	return s.saveMetaUnlocked(meta)
}

// AddFontMeta adds font metadata, replacing any existing entry with the same filename.
func (s *ImageService) AddFontMeta(fontMeta models.FontMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, err := s.loadMetaUnlocked()
	if err != nil {
		return err
	}
	found := false
	for i, f := range meta.Fonts {
		if f.Filename == fontMeta.Filename {
			meta.Fonts[i] = fontMeta
			found = true
			break
		}
	}
	if !found {
		meta.Fonts = append(meta.Fonts, fontMeta)
	}
	return s.saveMetaUnlocked(meta)
}

// RemoveFontMeta removes font metadata by filename.
func (s *ImageService) RemoveFontMeta(filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, err := s.loadMetaUnlocked()
	if err != nil {
		return err
	}
	filtered := make([]models.FontMeta, 0, len(meta.Fonts))
	for _, f := range meta.Fonts {
		if f.Filename != filename {
			filtered = append(filtered, f)
		}
	}
	meta.Fonts = filtered
	return s.saveMetaUnlocked(meta)
}

// --- Thumbnail generation ---

// generateThumbnail creates a PNG thumbnail of maxDim x maxDim (preserving aspect ratio).
func (s *ImageService) generateThumbnail(img image.Image, filename string) error {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Calculate thumbnail dimensions preserving aspect ratio
	thumbW, thumbH := w, h
	if w > thumbMaxDim || h > thumbMaxDim {
		if w > h {
			thumbW = thumbMaxDim
			thumbH = h * thumbMaxDim / w
		} else {
			thumbH = thumbMaxDim
			thumbW = w * thumbMaxDim / h
		}
	}

	// Skip thumbnail if image is already small enough
	if thumbW == w && thumbH == h {
		thumbW, thumbH = w, h
	}

	if thumbW < 1 {
		thumbW = 1
	}
	if thumbH < 1 {
		thumbH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, thumbW, thumbH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, xdraw.Over, nil)

	outPath := filepath.Join(s.thumbsDir, filename)
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, dst)
}

// GetThumbPath returns the full path to a thumbnail file.
func (s *ImageService) GetThumbPath(filename string) (string, error) {
	safe, err := sanitize(filename)
	if err != nil {
		return "", err
	}
	p := filepath.Join(s.thumbsDir, safe)
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return "", ErrFileNotFound
		}
		return "", err
	}
	return p, nil
}

// --- Image operations ---

// ListImages returns metadata for all image files in the images directory.
func (s *ImageService) ListImages() ([]models.FileInfo, error) {
	entries, err := os.ReadDir(s.imagesDir)
	if err != nil {
		return []models.FileInfo{}, nil
	}
	result := []models.FileInfo{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, models.FileInfo{
			Name: e.Name(),
			Size: info.Size(),
		})
	}
	return result, nil
}

// ListImagesWithMeta returns a paginated, sorted, and filtered list of image metadata.
func (s *ImageService) ListImagesWithMeta(sortBy, search string, page, limit int) (models.ImageListResponse, error) {
	s.mu.RLock()
	meta, err := s.loadMetaUnlocked()
	s.mu.RUnlock()
	if err != nil {
		return models.ImageListResponse{}, err
	}

	// Filter by search term
	filtered := meta.Images
	if search != "" {
		searchLower := strings.ToLower(search)
		var matched []models.ImageMeta
		for _, img := range filtered {
			if strings.Contains(strings.ToLower(img.Filename), searchLower) ||
				strings.Contains(strings.ToLower(img.OrigName), searchLower) {
				matched = append(matched, img)
			}
		}
		filtered = matched
	}

	// Sort
	switch sortBy {
	case "date_asc":
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].UploadedAt.Before(filtered[j].UploadedAt)
		})
	case "name_asc":
		sort.Slice(filtered, func(i, j int) bool {
			return strings.ToLower(filtered[i].Filename) < strings.ToLower(filtered[j].Filename)
		})
	case "name_desc":
		sort.Slice(filtered, func(i, j int) bool {
			return strings.ToLower(filtered[i].Filename) > strings.ToLower(filtered[j].Filename)
		})
	case "size_desc":
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Size > filtered[j].Size
		})
	case "size_asc":
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Size < filtered[j].Size
		})
	case "width_desc":
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Width > filtered[j].Width
		})
	default: // date_desc
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].UploadedAt.After(filtered[j].UploadedAt)
		})
	}

	total := len(filtered)

	// Pagination defaults
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	pageItems := filtered[start:end]
	if pageItems == nil {
		pageItems = []models.ImageMeta{}
	}

	return models.ImageListResponse{
		Images: pageItems,
		Total:  total,
		Page:   page,
		Limit:  limit,
	}, nil
}

// ListFontsWithMeta returns font metadata, optionally filtered by search.
func (s *ImageService) ListFontsWithMeta(search string) ([]models.FontMeta, error) {
	s.mu.RLock()
	meta, err := s.loadMetaUnlocked()
	s.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	if search == "" {
		return meta.Fonts, nil
	}

	searchLower := strings.ToLower(search)
	var matched []models.FontMeta
	for _, f := range meta.Fonts {
		if strings.Contains(strings.ToLower(f.Filename), searchLower) ||
			strings.Contains(strings.ToLower(f.OrigName), searchLower) {
			matched = append(matched, f)
		}
	}
	if matched == nil {
		matched = []models.FontMeta{}
	}
	return matched, nil
}

// GetImagePath returns the full path to the named image after sanitization.
func (s *ImageService) GetImagePath(name string) (string, error) {
	safe, err := sanitize(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.imagesDir, safe), nil
}

// SaveImage decodes an image from reader, re-encodes as PNG, and saves it.
// The filename is sanitized and forced to have a .png extension.
func (s *ImageService) SaveImage(name string, r io.Reader) error {
	safe, err := sanitize(name)
	if err != nil {
		return err
	}

	// Force .png extension
	ext := filepath.Ext(safe)
	baseName := strings.TrimSuffix(safe, ext)
	safe = baseName + ".png"

	// Limit reader to max size
	limited := io.LimitReader(r, maxImageSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(data) > maxImageSize {
		return ErrFileTooLarge
	}

	// Decode the image (supports PNG, JPEG via registered decoders)
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return ErrInvalidFileType
	}

	// Re-encode as PNG
	outPath := filepath.Join(s.imagesDir, safe)
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, img)
}

// SaveImageWithMeta decodes an image, saves as PNG, generates thumbnail, and stores metadata.
// Returns the saved filename and ImageMeta.
func (s *ImageService) SaveImageWithMeta(origName string, r io.Reader) (string, models.ImageMeta, error) {
	safe, err := sanitize(origName)
	if err != nil {
		return "", models.ImageMeta{}, err
	}

	// Force .png extension
	ext := filepath.Ext(safe)
	baseName := strings.TrimSuffix(safe, ext)
	safe = baseName + ".png"

	// Limit reader to max size
	limited := io.LimitReader(r, maxImageSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", models.ImageMeta{}, err
	}
	if len(data) > maxImageSize {
		return "", models.ImageMeta{}, ErrFileTooLarge
	}

	// Decode the image
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", models.ImageMeta{}, ErrInvalidFileType
	}

	bounds := img.Bounds()

	// Re-encode as PNG
	outPath := filepath.Join(s.imagesDir, safe)
	f, err := os.Create(outPath)
	if err != nil {
		return "", models.ImageMeta{}, err
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		return "", models.ImageMeta{}, err
	}

	// Get saved file size
	fi, err := os.Stat(outPath)
	if err != nil {
		return "", models.ImageMeta{}, err
	}

	// Generate thumbnail
	hasThumb := true
	if err := s.generateThumbnail(img, safe); err != nil {
		slog.Warn("failed to generate thumbnail", "filename", safe, "error", err)
		hasThumb = false
	}

	imgMeta := models.ImageMeta{
		Filename:   safe,
		OrigName:   origName,
		UploadedAt: time.Now().UTC(),
		Size:       fi.Size(),
		Width:      bounds.Dx(),
		Height:     bounds.Dy(),
		MimeType:   "image/png",
		HasThumb:   hasThumb,
	}

	if err := s.AddImageMeta(imgMeta); err != nil {
		slog.Warn("failed to save image metadata", "error", err)
	}

	return safe, imgMeta, nil
}

// DeleteImage removes the named image file.
func (s *ImageService) DeleteImage(name string) error {
	safe, err := sanitize(name)
	if err != nil {
		return err
	}
	p := filepath.Join(s.imagesDir, safe)
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return ErrFileNotFound
		}
		return err
	}
	return nil
}

// DeleteImageWithMeta removes image, thumbnail, and metadata.
func (s *ImageService) DeleteImageWithMeta(name string) error {
	safe, err := sanitize(name)
	if err != nil {
		return err
	}

	// Delete main image
	p := filepath.Join(s.imagesDir, safe)
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return ErrFileNotFound
		}
		return err
	}

	// Delete thumbnail (best effort)
	thumbPath := filepath.Join(s.thumbsDir, safe)
	if err := os.Remove(thumbPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to delete thumbnail", "filename", safe, "error", err)
	}

	// Remove metadata (best effort)
	if err := s.RemoveImageMeta(safe); err != nil {
		slog.Warn("failed to remove image metadata", "filename", safe, "error", err)
	}

	return nil
}

// --- Font operations ---

// ListFonts returns metadata for all .ttf/.otf files in the fonts directory.
func (s *ImageService) ListFonts() ([]models.FileInfo, error) {
	entries, err := os.ReadDir(s.fontsDir)
	if err != nil {
		return []models.FileInfo{}, nil
	}
	result := []models.FileInfo{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".ttf" && ext != ".otf" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, models.FileInfo{
			Name: e.Name(),
			Size: info.Size(),
		})
	}
	return result, nil
}

// GetFontPath returns the full path to the named font file.
func (s *ImageService) GetFontPath(name string) (string, error) {
	safe, err := sanitize(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.fontsDir, safe), nil
}

// SaveFont saves a font file (.ttf or .otf) from the reader.
func (s *ImageService) SaveFont(name string, r io.Reader) error {
	safe, err := sanitize(name)
	if err != nil {
		return err
	}

	ext := strings.ToLower(filepath.Ext(safe))
	if ext != ".ttf" && ext != ".otf" {
		return ErrInvalidFileType
	}

	// Limit reader to max size
	limited := io.LimitReader(r, maxFontSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(data) > maxFontSize {
		return ErrFileTooLarge
	}

	outPath := filepath.Join(s.fontsDir, safe)
	return os.WriteFile(outPath, data, 0644)
}

// SaveFontWithMeta saves a font and stores metadata.
func (s *ImageService) SaveFontWithMeta(origName string, r io.Reader) (string, models.FontMeta, error) {
	safe, err := sanitize(origName)
	if err != nil {
		return "", models.FontMeta{}, err
	}

	ext := strings.ToLower(filepath.Ext(safe))
	if ext != ".ttf" && ext != ".otf" {
		return "", models.FontMeta{}, ErrInvalidFileType
	}

	limited := io.LimitReader(r, maxFontSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", models.FontMeta{}, err
	}
	if len(data) > maxFontSize {
		return "", models.FontMeta{}, ErrFileTooLarge
	}

	outPath := filepath.Join(s.fontsDir, safe)
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return "", models.FontMeta{}, err
	}

	fontMeta := models.FontMeta{
		Filename:   safe,
		OrigName:   origName,
		UploadedAt: time.Now().UTC(),
		Size:       int64(len(data)),
	}

	if err := s.AddFontMeta(fontMeta); err != nil {
		slog.Warn("failed to save font metadata", "error", err)
	}

	return safe, fontMeta, nil
}

// DeleteFont removes the named font file.
func (s *ImageService) DeleteFont(name string) error {
	safe, err := sanitize(name)
	if err != nil {
		return err
	}
	p := filepath.Join(s.fontsDir, safe)
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return ErrFileNotFound
		}
		return err
	}
	return nil
}

// DeleteFontWithMeta removes font file and metadata.
func (s *ImageService) DeleteFontWithMeta(name string) error {
	safe, err := sanitize(name)
	if err != nil {
		return err
	}

	p := filepath.Join(s.fontsDir, safe)
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return ErrFileNotFound
		}
		return err
	}

	if err := s.RemoveFontMeta(safe); err != nil {
		slog.Warn("failed to remove font metadata", "filename", safe, "error", err)
	}

	return nil
}
