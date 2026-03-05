package services

import "e-ink-picture/server/internal/models"

type PreviewService struct {
	design  *DesignService
	weather *WeatherService
	image   *ImageService
	dataDir string
}

func NewPreviewService(d *DesignService, w *WeatherService, i *ImageService, dataDir string) *PreviewService {
	return &PreviewService{design: d, weather: w, image: i, dataDir: dataDir}
}

func (s *PreviewService) Render(design *models.Design) ([]byte, error) {
	return nil, nil
}

func (s *PreviewService) RenderActive() ([]byte, error) {
	return nil, nil
}
