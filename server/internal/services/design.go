package services

import "e-ink-picture/server/internal/models"

type DesignService struct {
	dataDir string
}

func NewDesignService(dataDir string) *DesignService {
	return &DesignService{dataDir: dataDir}
}

func (s *DesignService) List() ([]models.DesignMeta, error) {
	return nil, nil
}

func (s *DesignService) Get(name string) (*models.Design, error) {
	return nil, nil
}

func (s *DesignService) Save(name string, design *models.Design) error {
	return nil
}

func (s *DesignService) Delete(name string) error {
	return nil
}

func (s *DesignService) Clone(source, target string) error {
	return nil
}

func (s *DesignService) SetActive(name string) error {
	return nil
}

func (s *DesignService) GetActive() (*models.Design, error) {
	return nil, nil
}

func (s *DesignService) GetActiveName() (string, error) {
	return "", nil
}
