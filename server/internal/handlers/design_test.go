package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"e-ink-picture/server/internal/models"
	"e-ink-picture/server/internal/services"
)

func setupDesignTestServices(t *testing.T) (*DesignHandler, *PreviewHandler, *SettingsHandler, string) {
	t.Helper()
	tmpDir := t.TempDir()

	for _, sub := range []string{"designs", "uploaded_images", "fonts", "weather_styles"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	settingsData := `{"display_type":"waveshare_7in5_V2","refresh_interval":3600}`
	os.WriteFile(filepath.Join(tmpDir, "settings.json"), []byte(settingsData), 0644)

	designSvc := services.NewDesignService(tmpDir)
	imageSvc := services.NewImageService(tmpDir)
	weatherSvc := services.NewWeatherService("", "", tmpDir)
	settingsSvc := services.NewSettingsService(tmpDir)
	previewSvc := services.NewPreviewService(designSvc, weatherSvc, imageSvc, settingsSvc, tmpDir)

	designH := NewDesignHandler(designSvc, previewSvc)
	previewH := NewPreviewHandler(previewSvc, designSvc)
	settingsH := NewSettingsHandler(settingsSvc)

	return designH, previewH, settingsH, tmpDir
}

func makeDesign(name string, elements []models.Element) models.DesignV2 {
	vis := true
	for i := range elements {
		if elements[i].Visible == nil {
			elements[i].Visible = &vis
		}
	}
	return models.DesignV2{
		Name:    name,
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: elements,
	}
}

// TestDesignCRUDWorkflow tests the full create-read-update-delete cycle.
func TestDesignCRUDWorkflow(t *testing.T) {
	designH, _, _, _ := setupDesignTestServices(t)

	// 1. List designs — should be empty
	req := httptest.NewRequest("GET", "/designs", nil)
	rec := httptest.NewRecorder()
	designH.List(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("List: expected 200, got %d", rec.Code)
	}
	var list []models.DesignV2
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 0 {
		t.Fatalf("List: expected 0 designs, got %d", len(list))
	}

	// 2. Save a new design
	design := makeDesign("Test Design", []models.Element{
		{
			ID: "t1", Type: "text", X: 50, Y: 50, Width: 200, Height: 60, ZIndex: 0,
			Properties: map[string]any{"text": "Hello", "fontSize": float64(24), "color": "#000000"},
		},
	})
	body := map[string]any{
		"name":        design.Name,
		"version":     design.Version,
		"canvas":      design.Canvas,
		"elements":    design.Elements,
		"save_as_new": true,
	}
	raw, _ := json.Marshal(body)
	req = httptest.NewRequest("POST", "/update_design", bytes.NewReader(raw))
	rec = httptest.NewRecorder()
	designH.Update(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Create: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. List again — should have 1 design
	req = httptest.NewRequest("GET", "/designs", nil)
	rec = httptest.NewRecorder()
	designH.List(rec, req)
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("List: expected 1 design, got %d", len(list))
	}
	if list[0].Name != "Test Design" {
		t.Errorf("List: expected name 'Test Design', got '%s'", list[0].Name)
	}

	// 4. Get by name
	req = httptest.NewRequest("GET", "/design?name=Test+Design", nil)
	rec = httptest.NewRecorder()
	designH.GetByName(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetByName: expected 200, got %d", rec.Code)
	}

	// 5. Get active
	req = httptest.NewRequest("GET", "/active_design", nil)
	rec = httptest.NewRecorder()
	designH.GetActive(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetActive: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 6. Update existing design (add an element)
	body["save_as_new"] = false
	body["elements"] = append(design.Elements, models.Element{
		ID: "t2", Type: "text", X: 100, Y: 200, Width: 150, Height: 40, ZIndex: 1,
		Visible:    design.Elements[0].Visible,
		Properties: map[string]any{"text": "World", "fontSize": float64(18), "color": "#000000"},
	})
	raw, _ = json.Marshal(body)
	req = httptest.NewRequest("POST", "/update_design", bytes.NewReader(raw))
	rec = httptest.NewRecorder()
	designH.Update(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Update: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 7. Verify update — get active should have 2 elements
	req = httptest.NewRequest("GET", "/active_design", nil)
	rec = httptest.NewRecorder()
	designH.GetActive(rec, req)
	var updated models.DesignV2
	json.NewDecoder(rec.Body).Decode(&updated)
	if len(updated.Elements) != 2 {
		t.Errorf("Update: expected 2 elements, got %d", len(updated.Elements))
	}

	// 8. Clone design — wait 1s to avoid timestamp collision in filename
	time.Sleep(1 * time.Second)
	cloneBody, _ := json.Marshal(map[string]string{"name": "Test Design"})
	req = httptest.NewRequest("POST", "/clone_design", bytes.NewReader(cloneBody))
	rec = httptest.NewRecorder()
	designH.Clone(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Clone: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 9. List should have 2 designs now
	req = httptest.NewRequest("GET", "/designs", nil)
	rec = httptest.NewRecorder()
	designH.List(rec, req)
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 2 {
		t.Fatalf("List after clone: expected 2, got %d", len(list))
	}

	// 10. Delete original
	delBody, _ := json.Marshal(map[string]string{"name": "Test Design"})
	req = httptest.NewRequest("DELETE", "/delete_design", bytes.NewReader(delBody))
	rec = httptest.NewRecorder()
	designH.Delete(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Delete: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 11. List should have 1 design (the clone)
	req = httptest.NewRequest("GET", "/designs", nil)
	rec = httptest.NewRecorder()
	designH.List(rec, req)
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("List after delete: expected 1, got %d", len(list))
	}
}

// TestDesignGetByNameMissing tests 404 for nonexistent design.
func TestDesignGetByNameMissing(t *testing.T) {
	designH, _, _, _ := setupDesignTestServices(t)

	req := httptest.NewRequest("GET", "/design?name=nonexistent", nil)
	rec := httptest.NewRecorder()
	designH.GetByName(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for missing design, got %d", rec.Code)
	}
}

// TestDesignGetByNameEmpty tests 400 when name param is missing.
func TestDesignGetByNameEmpty(t *testing.T) {
	designH, _, _, _ := setupDesignTestServices(t)

	req := httptest.NewRequest("GET", "/design", nil)
	rec := httptest.NewRecorder()
	designH.GetByName(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty name, got %d", rec.Code)
	}
}

// TestDesignDeleteNonexistent tests 404 when deleting nonexistent design.
func TestDesignDeleteNonexistent(t *testing.T) {
	designH, _, _, _ := setupDesignTestServices(t)

	body, _ := json.Marshal(map[string]string{"name": "nonexistent"})
	req := httptest.NewRequest("DELETE", "/delete_design", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	designH.Delete(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for deleting missing design, got %d", rec.Code)
	}
}

// TestPreviewLiveMultipleWidgets tests rendering a design with multiple widget types.
func TestPreviewLiveMultipleWidgets(t *testing.T) {
	_, previewH, _, _ := setupDesignTestServices(t)

	vis := true
	design := models.DesignV2{
		Name:    "multi-widget-test",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID: "clock1", Type: "widget_clock", X: 10, Y: 10, Width: 200, Height: 80, ZIndex: 0, Visible: &vis,
				Properties: map[string]any{"format": "15:04", "color": "#000000", "fontSize": float64(48)},
			},
			{
				ID: "text1", Type: "text", X: 10, Y: 100, Width: 300, Height: 40, ZIndex: 1, Visible: &vis,
				Properties: map[string]any{"text": "Hello World", "fontSize": float64(24), "color": "#000000"},
			},
			{
				ID: "shape1", Type: "shape", X: 400, Y: 10, Width: 100, Height: 100, ZIndex: 2, Visible: &vis,
				Properties: map[string]any{"shapeType": "rectangle", "fill": "#FF0000", "stroke": "#000000", "strokeWidth": float64(2)},
			},
			{
				ID: "timer1", Type: "widget_timer", X: 400, Y: 200, Width: 200, Height: 60, ZIndex: 3, Visible: &vis,
				Properties: map[string]any{"label": "Next Event", "targetDate": "2030-01-01T00:00:00Z", "color": "#000000", "fontSize": float64(18)},
			},
		},
	}

	body, _ := json.Marshal(design)
	req := httptest.NewRequest("POST", "/api/preview_live", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	previewH.PreviewLive(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Expected Content-Type image/png, got %s", ct)
	}
	if rec.Body.Len() < 100 {
		t.Error("Response body too small for valid PNG")
	}
}

// TestPreviewLiveRawMode tests rendering without dithering.
func TestPreviewLiveRawMode(t *testing.T) {
	_, previewH, _, _ := setupDesignTestServices(t)

	vis := true
	design := models.DesignV2{
		Name:    "raw-test",
		Version: 2,
		Canvas:  models.CanvasConfig{Width: 800, Height: 480, Background: "#FFFFFF"},
		Elements: []models.Element{
			{
				ID: "t1", Type: "text", X: 50, Y: 50, Width: 200, Height: 60, ZIndex: 0, Visible: &vis,
				Properties: map[string]any{"text": "Raw Mode", "fontSize": float64(24), "color": "#000000"},
			},
		},
	}

	body, _ := json.Marshal(design)
	req := httptest.NewRequest("POST", "/api/preview_live?raw=true", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	previewH.PreviewLive(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Expected Content-Type image/png, got %s", ct)
	}
}

// TestPreviewSavedDesign tests the GET /preview endpoint with a saved design.
func TestPreviewSavedDesign(t *testing.T) {
	designH, previewH, _, _ := setupDesignTestServices(t)

	// Save a design first
	design := makeDesign("Preview Test", []models.Element{
		{
			ID: "t1", Type: "text", X: 0, Y: 0, Width: 800, Height: 480, ZIndex: 0,
			Properties: map[string]any{"text": "Full Screen", "fontSize": float64(48), "color": "#000000"},
		},
	})
	body := map[string]any{
		"name": design.Name, "version": design.Version,
		"canvas": design.Canvas, "elements": design.Elements,
		"save_as_new": true,
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/update_design", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	designH.Update(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Setup: save design failed: %d", rec.Code)
	}

	// Preview by name
	req = httptest.NewRequest("GET", "/preview?name=Preview+Test", nil)
	rec = httptest.NewRecorder()
	previewH.Preview(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Preview by name: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Expected Content-Type image/png, got %s", ct)
	}

	// Preview active
	req = httptest.NewRequest("GET", "/preview", nil)
	rec = httptest.NewRecorder()
	previewH.Preview(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Preview active: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestSettingsAndRefreshWorkflow tests settings read, trigger refresh, status, heartbeat.
func TestSettingsAndRefreshWorkflow(t *testing.T) {
	_, _, settingsH, tmpDir := setupDesignTestServices(t)

	// Write settings with a recent client refresh so should_refresh is initially false
	// Use 5 seconds ago so TriggerRefresh (which uses time.Now()) is clearly newer
	recentRefresh := time.Now().Add(-5 * time.Second).UTC().Format(time.RFC3339)
	settingsJSON := `{"display_type":"waveshare_7in5_V2","refresh_interval":3600,"last_client_refresh":"` + recentRefresh + `"}`
	os.WriteFile(filepath.Join(tmpDir, "settings.json"), []byte(settingsJSON), 0644)

	// 1. Get settings
	req := httptest.NewRequest("GET", "/settings", nil)
	rec := httptest.NewRecorder()
	settingsH.GetSettings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetSettings: expected 200, got %d", rec.Code)
	}

	// 2. Check refresh status — should be false (just refreshed)
	req = httptest.NewRequest("GET", "/api/refresh_status", nil)
	rec = httptest.NewRecorder()
	settingsH.RefreshStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("RefreshStatus: expected 200, got %d", rec.Code)
	}
	var status map[string]any
	json.NewDecoder(rec.Body).Decode(&status)
	if status["should_refresh"] == true {
		t.Error("Expected should_refresh false initially")
	}

	// 3. Trigger refresh
	req = httptest.NewRequest("POST", "/api/trigger_refresh", nil)
	rec = httptest.NewRecorder()
	settingsH.TriggerRefresh(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("TriggerRefresh: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Check refresh status — should be true
	req = httptest.NewRequest("GET", "/api/refresh_status", nil)
	rec = httptest.NewRecorder()
	settingsH.RefreshStatus(rec, req)
	json.NewDecoder(rec.Body).Decode(&status)
	if status["should_refresh"] != true {
		t.Error("Expected should_refresh true after trigger")
	}

	// 5. Client heartbeat — should clear refresh
	heartbeat, _ := json.Marshal(map[string]string{"status": "refreshed"})
	req = httptest.NewRequest("POST", "/api/client_heartbeat", bytes.NewReader(heartbeat))
	rec = httptest.NewRecorder()
	settingsH.ClientHeartbeat(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ClientHeartbeat: expected 200, got %d", rec.Code)
	}

	// 6. Check refresh status — should be false again
	req = httptest.NewRequest("GET", "/api/refresh_status", nil)
	rec = httptest.NewRecorder()
	settingsH.RefreshStatus(rec, req)
	json.NewDecoder(rec.Body).Decode(&status)
	if status["should_refresh"] == true {
		t.Error("Expected should_refresh false after heartbeat")
	}
}

// TestHealthEndpoint tests the health check handler.
func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	HealthCheck(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}
	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", resp["status"])
	}
}
