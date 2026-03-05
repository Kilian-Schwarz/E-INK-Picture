package services

import (
	"io"

	"e-ink-picture/server/internal/models"
)

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

func (s *ImageService) ListImages() ([]models.FileInfo, error) {
	return nil, nil
}

func (s *ImageService) GetImagePath(name string) (string, error) {
	return "", nil
}

func (s *ImageService) SaveImage(name string, r io.Reader) error {
	return nil
}

func (s *ImageService) DeleteImage(name string) error {
	return nil
}

func (s *ImageService) ListFonts() ([]models.FileInfo, error) {
	return nil, nil
}

func (s *ImageService) GetFontPath(name string) (string, error) {
	return "", nil
}

func (s *ImageService) SaveFont(name string, r io.Reader) error {
	return nil
}

func (s *ImageService) DeleteFont(name string) error {
	return nil
}
