package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"e-ink-picture/server/internal/models"
)

// v1OffsetX and v1OffsetY are the old canvas offsets used in v1 designs.
const (
	v1OffsetX = 200
	v1OffsetY = 160
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

// loadDesign reads and parses a design file. It auto-migrates v1 designs to v2.
func (s *DesignService) loadDesign(filename string) (*models.DesignV2, error) {
	data, err := os.ReadFile(filepath.Join(s.designsDir(), filename))
	if err != nil {
		return nil, err
	}

	// Try to detect version by checking for "version" field
	var probe struct {
		Version  int              `json:"version"`
		Elements []map[string]any `json:"elements"`
		Modules  []map[string]any `json:"modules"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, err
	}

	if probe.Version >= 2 && probe.Elements != nil {
		// v2 format
		var d models.DesignV2
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, err
		}
		d.Filename = filename
		ensureID(&d)
		ensureTimestamps(&d)
		return &d, nil
	}

	// v1 format - parse and migrate
	var v1 models.Design
	if err := json.Unmarshal(data, &v1); err != nil {
		return nil, err
	}
	v1.Filename = filename
	v2 := migrateV1ToV2(&v1)
	return v2, nil
}

// migrateV1ToV2 converts a v1 design (old module format) to v2 (element format).
func migrateV1ToV2(v1 *models.Design) *models.DesignV2 {
	canvasW := 800
	canvasH := 480
	if len(v1.Resolution) >= 2 {
		canvasW = v1.Resolution[0]
		canvasH = v1.Resolution[1]
	}

	elements := make([]models.Element, 0, len(v1.Modules))
	for i, m := range v1.Modules {
		elemType := mapV1TypeToV2(m.Type)

		// Convert offset-based coordinates to direct canvas coordinates
		x := float64(m.Position.X - v1OffsetX)
		y := float64(m.Position.Y - v1OffsetY)

		props := make(map[string]any)

		// Migrate style data to properties
		sd := m.StyleData
		if sd.Font != nil {
			props["fontFamily"] = *sd.Font
		}
		if sd.FontSize != nil {
			props["fontSize"] = *sd.FontSize
		}
		if sd.FontBold != nil && *sd.FontBold == "true" {
			props["bold"] = true
		}
		if sd.FontItalic != nil && *sd.FontItalic == "true" {
			props["italic"] = true
		}
		if sd.FontStrike != nil && *sd.FontStrike == "true" {
			props["strikethrough"] = true
		}
		if sd.TextAlign != nil {
			props["textAlign"] = *sd.TextAlign
		}
		if sd.TextColor != nil {
			props["color"] = *sd.TextColor
		}

		// Type-specific properties
		switch m.Type {
		case "text", "news":
			props["content"] = m.Content
		case "image":
			if sd.Image != nil {
				props["src"] = *sd.Image
			}
			if sd.CropX != nil {
				props["cropX"] = *sd.CropX
			}
			if sd.CropY != nil {
				props["cropY"] = *sd.CropY
			}
			if sd.CropW != nil {
				props["cropW"] = *sd.CropW
			}
			if sd.CropH != nil {
				props["cropH"] = *sd.CropH
			}
		case "datetime":
			if sd.DatetimeFormat != nil {
				props["format"] = *sd.DatetimeFormat
			} else {
				props["format"] = "YYYY-MM-DD HH:mm"
			}
		case "weather":
			if sd.Latitude != nil {
				props["latitude"] = *sd.Latitude
			}
			if sd.Longitude != nil {
				props["longitude"] = *sd.Longitude
			}
			if sd.LocationName != nil {
				props["location"] = *sd.LocationName
			}
			if sd.WeatherStyle != nil {
				props["style"] = *sd.WeatherStyle
			} else {
				props["style"] = "compact"
			}
		case "timer":
			if sd.TimerTarget != nil {
				props["targetDate"] = *sd.TimerTarget
			}
			if sd.TimerFormat != nil {
				props["format"] = *sd.TimerFormat
			}
		case "calendar":
			if sd.CalendarURL != nil {
				props["icalUrl"] = *sd.CalendarURL
			}
			if sd.MaxEvents != nil {
				props["maxEvents"] = *sd.MaxEvents
			}
		case "line":
			props["shape"] = "rectangle"
			props["fill"] = true
		}

		if sd.NewsHeadline != nil {
			props["title"] = *sd.NewsHeadline
		}

		elem := models.Element{
			ID:         fmt.Sprintf("migrated_%d", i),
			Type:       elemType,
			X:          x,
			Y:          y,
			Width:      float64(m.Size.Width),
			Height:     float64(m.Size.Height),
			Rotation:   0,
			ZIndex:     i,
			Locked:     false,
			Visible:    boolPtr(true),
			Properties: props,
		}
		elements = append(elements, elem)
	}

	return &models.DesignV2{
		Name:    v1.Name,
		Version: 2,
		Canvas: models.CanvasConfig{
			Width:      canvasW,
			Height:     canvasH,
			Background: "#FFFFFF",
		},
		Elements:  elements,
		Timestamp: v1.Timestamp,
		Active:    v1.Active,
		KeepAlive: v1.KeepAlive,
		Filename:  v1.Filename,
	}
}

// mapV1TypeToV2 maps old module type names to new element type names.
func mapV1TypeToV2(v1Type string) string {
	switch v1Type {
	case "text":
		return "text"
	case "image":
		return "image"
	case "weather":
		return "widget_weather"
	case "datetime":
		return "widget_clock"
	case "timer":
		return "widget_timer"
	case "calendar":
		return "widget_calendar"
	case "news":
		return "widget_news"
	case "line":
		return "shape"
	default:
		return "text"
	}
}

// saveDesign writes a design in v2 format to disk.
func (s *DesignService) saveDesign(d *models.DesignV2) error {
	// Ensure version is set
	if d.Version < 2 {
		d.Version = 2
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.designsDir(), d.Filename), data, 0644)
}

// loadAll reads all design files from the designs directory.
func (s *DesignService) loadAll() ([]*models.DesignV2, error) {
	files, err := s.listFiles()
	if err != nil {
		return nil, err
	}
	var designs []*models.DesignV2
	for _, fn := range files {
		d, err := s.loadDesign(fn)
		if err != nil {
			slog.Warn("failed to load design", "file", fn, "error", err)
			continue
		}
		designs = append(designs, d)
	}
	return designs, nil
}

// List returns lightweight metadata for all designs.
func (s *DesignService) List() ([]models.DesignV2Meta, error) {
	designs, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	metas := make([]models.DesignV2Meta, 0, len(designs))
	for _, d := range designs {
		metas = append(metas, models.DesignV2Meta{
			Name:   d.Name,
			Active: d.Active,
		})
	}
	return metas, nil
}

// ListFull returns the complete design objects for all designs.
func (s *DesignService) ListFull() ([]models.DesignV2, error) {
	designs, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	result := make([]models.DesignV2, 0, len(designs))
	for _, d := range designs {
		result = append(result, *d)
	}
	return result, nil
}

// Get finds a design by its name field.
func (s *DesignService) Get(name string) (*models.DesignV2, error) {
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
func (s *DesignService) Save(name string, design *models.DesignV2) error {
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
	var found *models.DesignV2
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
	// Deep copy elements
	clone.Elements = make([]models.Element, len(d.Elements))
	copy(clone.Elements, d.Elements)
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
func (s *DesignService) GetActive() (*models.DesignV2, error) {
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
		defaultDesign := &models.DesignV2{
			Name:    "Default Design",
			Version: 2,
			Canvas: models.CanvasConfig{
				Width:      800,
				Height:     480,
				Background: "#FFFFFF",
			},
			Elements:  []models.Element{},
			Timestamp: "initial",
			Active:    true,
			KeepAlive: false,
			Filename:  "design_default.json",
		}
		if err := s.saveDesign(defaultDesign); err != nil {
			return err
		}
	}
	return s.EnsureActive()
}

func boolPtr(v bool) *bool { return &v }

// generateID creates a short random hex ID.
func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// filenameToID derives a design ID from its filename.
func filenameToID(filename string) string {
	return strings.TrimSuffix(filename, ".json")
}

// historyDir returns the history directory for a given design ID.
func (s *DesignService) historyDir(designID string) string {
	return filepath.Join(s.dataDir, "designs", "history", designID)
}

const maxSnapshots = 50

// ensureID makes sure a design has an ID (derived from filename).
func ensureID(d *models.DesignV2) {
	if d.ID == "" && d.Filename != "" {
		d.ID = filenameToID(d.Filename)
	}
}

// ensureTimestamps sets created/updated timestamps if missing.
func ensureTimestamps(d *models.DesignV2) {
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = now
	}
}

// --- ID-based API methods ---

// GetByID finds a design by its ID (derived from filename).
func (s *DesignService) GetByID(id string) (*models.DesignV2, error) {
	designs, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	for _, d := range designs {
		ensureID(d)
		if d.ID == id {
			ensureTimestamps(d)
			return d, nil
		}
	}
	return nil, ErrDesignNotFound
}

// ListCards returns dashboard card metadata for all designs.
func (s *DesignService) ListCards() ([]models.DesignCardMeta, error) {
	designs, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	cards := make([]models.DesignCardMeta, 0, len(designs))
	for _, d := range designs {
		ensureID(d)
		ensureTimestamps(d)
		cards = append(cards, models.DesignCardMeta{
			ID:        d.ID,
			Name:      d.Name,
			Active:    d.Active,
			CreatedAt: d.CreatedAt,
			UpdatedAt: d.UpdatedAt,
			Elements:  len(d.Elements),
		})
	}
	// Sort by UpdatedAt descending (most recently edited first)
	sort.Slice(cards, func(i, j int) bool {
		return cards[i].UpdatedAt.After(cards[j].UpdatedAt)
	})
	return cards, nil
}

// CreateDesign creates a new design with a generated ID.
func (s *DesignService) CreateDesign(name string, elements []models.Element, canvas models.CanvasConfig) (*models.DesignV2, error) {
	if name == "" {
		return nil, ErrInvalidDesign
	}

	now := time.Now().UTC()
	id := "design_" + now.Format("2006-01-02_15-04-05") + "_" + generateID()[:6]
	filename := id + ".json"

	if canvas.Width == 0 {
		canvas.Width = 800
	}
	if canvas.Height == 0 {
		canvas.Height = 480
	}
	if canvas.Background == "" {
		canvas.Background = "#FFFFFF"
	}

	if elements == nil {
		elements = []models.Element{}
	}

	d := &models.DesignV2{
		ID:        id,
		Name:      name,
		Version:   2,
		Canvas:    canvas,
		Elements:  elements,
		Timestamp: now.Format("2006-01-02_15-04-05"),
		Active:    false,
		CreatedAt: now,
		UpdatedAt: now,
		Filename:  filename,
	}

	if err := s.saveDesign(d); err != nil {
		return nil, err
	}

	return d, nil
}

// UpdateDesignByID updates a design by its ID. Creates a history snapshot before saving.
func (s *DesignService) UpdateDesignByID(id string, name string, elements []models.Element, canvas models.CanvasConfig, keepAlive bool) (*models.DesignV2, error) {
	d, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Save history snapshot before overwriting
	s.saveHistorySnapshot(d)

	now := time.Now().UTC()
	if name != "" {
		d.Name = name
	}
	if elements != nil {
		d.Elements = elements
	}
	if canvas.Width > 0 {
		d.Canvas = canvas
	}
	d.KeepAlive = keepAlive
	d.UpdatedAt = now
	d.Version = 2

	if err := s.saveDesign(d); err != nil {
		return nil, err
	}

	return d, nil
}

// RenameDesign renames a design by ID.
func (s *DesignService) RenameDesign(id, newName string) (*models.DesignV2, error) {
	d, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	d.Name = newName
	d.UpdatedAt = time.Now().UTC()

	if err := s.saveDesign(d); err != nil {
		return nil, err
	}
	return d, nil
}

// DeleteByID removes a design file by ID and its history.
func (s *DesignService) DeleteByID(id string) error {
	d, err := s.GetByID(id)
	if err != nil {
		return err
	}

	if err := os.Remove(filepath.Join(s.designsDir(), d.Filename)); err != nil {
		return err
	}

	// Remove history directory (best effort)
	histDir := s.historyDir(id)
	if err := os.RemoveAll(histDir); err != nil {
		slog.Warn("failed to remove history", "id", id, "error", err)
	}

	return s.EnsureActive()
}

// ActivateByID sets a design as active by its ID.
func (s *DesignService) ActivateByID(id string) error {
	designs, err := s.loadAll()
	if err != nil {
		return err
	}
	found := false
	for _, d := range designs {
		ensureID(d)
		if d.ID == id {
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

// GetActiveDesign returns the currently active design (with ID ensured).
func (s *DesignService) GetActiveDesign() (*models.DesignV2, error) {
	d, err := s.GetActive()
	if err != nil {
		return nil, err
	}
	ensureID(d)
	ensureTimestamps(d)
	return d, nil
}

// DuplicateDesign creates a copy of a design with a new ID.
func (s *DesignService) DuplicateDesign(id string) (*models.DesignV2, error) {
	src, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	newID := "design_" + now.Format("2006-01-02_15-04-05") + "_" + generateID()[:6]

	clone := *src
	clone.ID = newID
	clone.Name = "Kopie von " + src.Name
	clone.Active = false
	clone.Timestamp = now.Format("2006-01-02_15-04-05")
	clone.CreatedAt = now
	clone.UpdatedAt = now
	clone.Filename = newID + ".json"

	// Deep copy elements
	clone.Elements = make([]models.Element, len(src.Elements))
	copy(clone.Elements, src.Elements)

	if err := s.saveDesign(&clone); err != nil {
		return nil, err
	}

	return &clone, nil
}

// --- History methods ---

// saveHistorySnapshot saves the current state of a design as a history entry.
func (s *DesignService) saveHistorySnapshot(d *models.DesignV2) {
	ensureID(d)
	histDir := s.historyDir(d.ID)
	if err := os.MkdirAll(histDir, 0755); err != nil {
		slog.Warn("failed to create history dir", "error", err)
		return
	}

	now := time.Now().UTC()
	ts := now.Format("2006-01-02T15-04-05")

	desc := fmt.Sprintf("%d elements", len(d.Elements))
	if d.Name != "" {
		desc = d.Name + " - " + desc
	}

	snapshot := models.HistorySnapshot{
		Timestamp:   ts,
		Description: desc,
		SavedAt:     now,
		Design:      *d,
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		slog.Warn("failed to marshal history snapshot", "error", err)
		return
	}

	filename := ts + ".json"
	if err := os.WriteFile(filepath.Join(histDir, filename), data, 0644); err != nil {
		slog.Warn("failed to write history snapshot", "error", err)
		return
	}

	// Enforce max snapshots (FIFO)
	s.pruneHistory(histDir)
}

// pruneHistory removes the oldest snapshots if there are more than maxSnapshots.
func (s *DesignService) pruneHistory(histDir string) {
	entries, err := os.ReadDir(histDir)
	if err != nil {
		return
	}

	var jsonFiles []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			jsonFiles = append(jsonFiles, e)
		}
	}

	if len(jsonFiles) <= maxSnapshots {
		return
	}

	// Sort by name (timestamps sort lexicographically)
	sort.Slice(jsonFiles, func(i, j int) bool {
		return jsonFiles[i].Name() < jsonFiles[j].Name()
	})

	// Remove oldest
	toRemove := len(jsonFiles) - maxSnapshots
	for i := 0; i < toRemove; i++ {
		p := filepath.Join(histDir, jsonFiles[i].Name())
		if err := os.Remove(p); err != nil {
			slog.Warn("failed to prune history", "file", p, "error", err)
		}
	}
}

// ListHistory returns history entries for a design.
func (s *DesignService) ListHistory(designID string) ([]models.HistoryEntry, error) {
	histDir := s.historyDir(designID)
	entries, err := os.ReadDir(histDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []models.HistoryEntry{}, nil
		}
		return nil, err
	}

	var result []models.HistoryEntry
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		ts := strings.TrimSuffix(e.Name(), ".json")

		// Try to read description from snapshot
		desc := ""
		data, err := os.ReadFile(filepath.Join(histDir, e.Name()))
		if err == nil {
			var snap models.HistorySnapshot
			if json.Unmarshal(data, &snap) == nil {
				desc = snap.Description
			}
		}

		result = append(result, models.HistoryEntry{
			Timestamp:   ts,
			Description: desc,
			Size:        info.Size(),
		})
	}

	// Sort newest first
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp > result[j].Timestamp
	})

	return result, nil
}

// GetHistorySnapshot returns a specific history snapshot.
func (s *DesignService) GetHistorySnapshot(designID, timestamp string) (*models.HistorySnapshot, error) {
	histDir := s.historyDir(designID)
	filename := timestamp + ".json"
	data, err := os.ReadFile(filepath.Join(histDir, filename))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrDesignNotFound
		}
		return nil, err
	}

	var snap models.HistorySnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// RestoreHistorySnapshot restores a design from a history snapshot.
// It first saves the current state as a new snapshot, then overwrites with the old state.
func (s *DesignService) RestoreHistorySnapshot(designID, timestamp string) (*models.DesignV2, error) {
	// Get current design
	current, err := s.GetByID(designID)
	if err != nil {
		return nil, err
	}

	// Get the snapshot to restore
	snap, err := s.GetHistorySnapshot(designID, timestamp)
	if err != nil {
		return nil, err
	}

	// Save current state as new snapshot first
	s.saveHistorySnapshot(current)

	// Restore: overwrite with snapshot data but keep ID/filename/active status
	restored := snap.Design
	restored.ID = current.ID
	restored.Filename = current.Filename
	restored.Active = current.Active
	restored.UpdatedAt = time.Now().UTC()
	restored.Version = 2

	if err := s.saveDesign(&restored); err != nil {
		return nil, err
	}

	return &restored, nil
}

// GetPropString extracts a string property from element properties.
func GetPropString(props map[string]any, key, fallback string) string {
	if v, ok := props[key]; ok {
		switch s := v.(type) {
		case string:
			return s
		case float64:
			return strconv.FormatFloat(s, 'f', -1, 64)
		}
	}
	return fallback
}

// GetPropFloat extracts a float64 property from element properties.
func GetPropFloat(props map[string]any, key string, fallback float64) float64 {
	if v, ok := props[key]; ok {
		switch f := v.(type) {
		case float64:
			return f
		case string:
			if val, err := strconv.ParseFloat(f, 64); err == nil {
				return val
			}
		}
	}
	return fallback
}

// GetPropInt extracts an int property from element properties.
func GetPropInt(props map[string]any, key string, fallback int) int {
	if v, ok := props[key]; ok {
		switch f := v.(type) {
		case float64:
			return int(f)
		case string:
			if val, err := strconv.Atoi(f); err == nil {
				return val
			}
		}
	}
	return fallback
}

// GetPropBool extracts a bool property from element properties.
func GetPropBool(props map[string]any, key string, fallback bool) bool {
	if v, ok := props[key]; ok {
		switch b := v.(type) {
		case bool:
			return b
		case string:
			return b == "true"
		}
	}
	return fallback
}
