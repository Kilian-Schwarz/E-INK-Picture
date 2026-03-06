package services

import (
	"errors"
	"bytes"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	// Register image decoders for JPEG and PNG.
	_ "image/jpeg"

	"e-ink-picture/server/internal/models"
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
)

// validFilenameRe allows alphanumeric characters, hyphens, underscores, dots, and parentheses.
var validFilenameRe = regexp.MustCompile(`^[a-zA-Z0-9\-_.()\s]+$`)

type ImageService struct {
	imagesDir string
	fontsDir  string
}

func NewImageService(dataDir string) *ImageService {
	return &ImageService{
		imagesDir: dataDir + "/uploaded_images",
		fontsDir:  dataDir + "/fonts",
	}
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

// ListImages returns metadata for all .png files in the images directory.
func (s *ImageService) ListImages() ([]models.FileInfo, error) {
	entries, err := os.ReadDir(s.imagesDir)
	if err != nil {
		return nil, err
	}
	var result []models.FileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.ToLower(filepath.Ext(e.Name())) != ".png" {
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

// ListFonts returns metadata for all .ttf/.otf files in the fonts directory.
func (s *ImageService) ListFonts() ([]models.FileInfo, error) {
	entries, err := os.ReadDir(s.fontsDir)
	if err != nil {
		return nil, err
	}
	var result []models.FileInfo
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
