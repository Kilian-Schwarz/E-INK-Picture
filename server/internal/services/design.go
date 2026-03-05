package services

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"e-ink-picture/server/internal/models"
)

var (
	ErrDesignNotFound = errors.New("design not found")
	ErrInvalidDesign  = errors.New("invalid design data")
)

type DesignService struct {
	dataDir string
}

func NewDesignService(dataDir string) *DesignService {
	return &DesignService{dataDir: dataDir}
}

// designsDir returns the path to the designs directory.
func (s *DesignService) designsDir() string {
	return filepath.Join(s.dataDir, "designs")
}

// listFiles returns all .json filenames in the designs directory, sorted.
func (s *DesignService) listFiles() ([]string, error) {
	entries, err := os.ReadDir(s.designsDir())
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

// loadDesign reads and parses a design file, setting Filename on the result.
func (s *DesignService) loadDesign(filename string) (*models.Design, error) {
	data, err := os.ReadFile(filepath.Join(s.designsDir(), filename))
	if err != nil {
		return nil, err
	}
	var d models.Design
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	d.Filename = filename
	return &d, nil
}

// saveDesign writes a design to disk. The Filename field is not written to JSON.
func (s *DesignService) saveDesign(d *models.Design) error {
	// Marshal without the filename field in JSON output
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.designsDir(), d.Filename), data, 0644)
}

// loadAll reads all design files from the designs directory.
func (s *DesignService) loadAll() ([]*models.Design, error) {
	files, err := s.listFiles()
	if err != nil {
		return nil, err
	}
	var designs []*models.Design
	for _, fn := range files {
		d, err := s.loadDesign(fn)
		if err != nil {
			continue
		}
		designs = append(designs, d)
	}
	return designs, nil
}

// List returns lightweight metadata for all designs.
func (s *DesignService) List() ([]models.DesignMeta, error) {
	designs, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	metas := make([]models.DesignMeta, 0, len(designs))
	for _, d := range designs {
		metas = append(metas, models.DesignMeta{
			Name:   d.Name,
			Active: d.Active,
		})
	}
	return metas, nil
}

// ListFull returns the complete design objects for all designs.
func (s *DesignService) ListFull() ([]models.Design, error) {
	designs, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	result := make([]models.Design, 0, len(designs))
	for _, d := range designs {
		result = append(result, *d)
	}
	return result, nil
}

// Get finds a design by its name field.
func (s *DesignService) Get(name string) (*models.Design, error) {
	designs, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	for _, d := range designs {
		if d.Name == name {
			return d, nil
		}
	}
	return nil, ErrDesignNotFound
}

// Save writes a design to disk. If design.Filename is empty, a timestamped
// filename is generated.
func (s *DesignService) Save(name string, design *models.Design) error {
	if design == nil {
		return ErrInvalidDesign
	}
	if design.Filename == "" {
		ts := time.Now().Format("2006-01-02_15-04-05")
		design.Filename = "design_" + ts + ".json"
	}
	design.Name = name
	return s.saveDesign(design)
}

// Delete removes a design file by name and ensures one design stays active.
func (s *DesignService) Delete(name string) error {
	designs, err := s.loadAll()
	if err != nil {
		return err
	}
	var found *models.Design
	for _, d := range designs {
		if d.Name == name {
			found = d
			break
		}
	}
	if found == nil {
		return ErrDesignNotFound
	}
	if err := os.Remove(filepath.Join(s.designsDir(), found.Filename)); err != nil {
		return err
	}
	return s.EnsureActive()
}

// Clone duplicates a design with a new name.
func (s *DesignService) Clone(source, target string) error {
	d, err := s.Get(source)
	if err != nil {
		return err
	}
	ts := time.Now().Format("2006-01-02_15-04-05")
	clone := *d
	clone.Name = target
	clone.Timestamp = ts
	clone.Active = false
	clone.Filename = "design_" + ts + ".json"
	return s.saveDesign(&clone)
}

// SetActive sets the given design as active and deactivates all others.
func (s *DesignService) SetActive(name string) error {
	designs, err := s.loadAll()
	if err != nil {
		return err
	}
	found := false
	for _, d := range designs {
		if d.Name == name {
			d.Active = true
			found = true
		} else {
			d.Active = false
		}
		if err := s.saveDesign(d); err != nil {
			return err
		}
	}
	if !found {
		return ErrDesignNotFound
	}
	return nil
}

// GetActive returns the currently active design.
func (s *DesignService) GetActive() (*models.Design, error) {
	designs, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	for _, d := range designs {
		if d.Active {
			return d, nil
		}
	}
	return nil, ErrDesignNotFound
}

// GetActiveName returns just the name of the active design.
func (s *DesignService) GetActiveName() (string, error) {
	d, err := s.GetActive()
	if err != nil {
		return "", err
	}
	return d.Name, nil
}

// EnsureActive ensures at least one design is marked active.
// If no design is active, the first one (sorted) is set active.
func (s *DesignService) EnsureActive() error {
	designs, err := s.loadAll()
	if err != nil {
		return err
	}
	if len(designs) == 0 {
		return nil
	}
	for _, d := range designs {
		if d.Active {
			return nil
		}
	}
	// No active design found; activate the first one.
	designs[0].Active = true
	return s.saveDesign(designs[0])
}

// EnsureDesignExists creates a default design if none exist, then ensures
// one is active.
func (s *DesignService) EnsureDesignExists() error {
	files, err := s.listFiles()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		defaultDesign := &models.Design{
			Modules:    []models.Module{},
			Resolution: []int{800, 480},
			Name:       "Default Design",
			Timestamp:  "initial",
			Active:     true,
			KeepAlive:  false,
			Filename:   "design_default.json",
		}
		if err := s.saveDesign(defaultDesign); err != nil {
			return err
		}
	}
	return s.EnsureActive()
}
