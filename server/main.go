package main

import (
	"context"
	"embed"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"e-ink-picture/server/internal/config"
	"e-ink-picture/server/internal/handlers"
	"e-ink-picture/server/internal/middleware"
	"e-ink-picture/server/internal/services"
)

//go:embed static/*
var staticFS embed.FS

//go:embed templates/*
var templateFS embed.FS

func main() {
	cfg := config.Load()

	// Ensure data directories exist
	dataDirs := []string{
		cfg.DataDir + "/designs",
		cfg.DataDir + "/uploaded_images",
		cfg.DataDir + "/uploaded_images/thumbs",
		cfg.DataDir + "/fonts",
		cfg.DataDir + "/weather_styles",
		cfg.DataDir + "/designs/history",
	}
	for _, dir := range dataDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			slog.Error("failed to create directory", "dir", dir, "error", err)
			os.Exit(1)
		}
	}

	// Parse templates
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		slog.Error("failed to parse templates", "error", err)
		os.Exit(1)
	}

	// Create services
	designSvc := services.NewDesignService(cfg.DataDir)
	imageSvc := services.NewImageService(cfg.DataDir)
	weatherSvc := services.NewWeatherService(cfg.WeatherAPIKey, cfg.WeatherLocation, cfg.DataDir)
	settingsSvc := services.NewSettingsService(cfg.DataDir)
	previewSvc := services.NewPreviewService(designSvc, weatherSvc, imageSvc, settingsSvc, cfg.DataDir)

	// Ensure at least one design exists (like Python's ensure_active_design on startup)
	if err := designSvc.EnsureDesignExists(); err != nil {
		slog.Error("failed to ensure design exists", "error", err)
		os.Exit(1)
	}

	// Create handlers
	designH := handlers.NewDesignHandler(designSvc, previewSvc)
	mediaH := handlers.NewMediaHandler(imageSvc)
	weatherH := handlers.NewWeatherHandler(weatherSvc)
	previewH := handlers.NewPreviewHandler(previewSvc, designSvc)
	displayH := handlers.NewDisplayHandler(cfg.EInkClientURL)
	settingsH := handlers.NewSettingsHandler(settingsSvc)
	widgetH := handlers.NewWidgetHandler(weatherSvc)

	// Setup router
	mux := http.NewServeMux()

	// Root redirect to designer
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/designer", http.StatusFound)
	})

	// Designer UI
	mux.HandleFunc("GET /designer", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "designer.html", nil); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
		}
	})

	// Media page (same as designer — media is integrated as modal/tab)
	mux.HandleFunc("GET /media", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/designer#media-images", http.StatusFound)
	})

	// Design endpoints
	mux.HandleFunc("GET /design", designH.GetActive)
	mux.HandleFunc("GET /designs", designH.List)
	mux.HandleFunc("GET /get_design_by_name", designH.GetByName)
	mux.HandleFunc("POST /set_active_design", designH.SetActive)
	mux.HandleFunc("POST /update_design", designH.Update)
	mux.HandleFunc("POST /clone_design", designH.Clone)
	mux.HandleFunc("POST /delete_design", designH.Delete)

	// Media endpoints (legacy)
	mux.HandleFunc("POST /upload_image", mediaH.Upload)
	mux.HandleFunc("GET /images_all", mediaH.ListImages)
	mux.HandleFunc("GET /image/{filename}", mediaH.GetImage)
	mux.HandleFunc("POST /delete_image", mediaH.DeleteImage)
	mux.HandleFunc("GET /fonts_all", mediaH.ListFonts)
	mux.HandleFunc("GET /font/{filename}", mediaH.GetFont)

	// Design Management API (new)
	mux.HandleFunc("GET /api/designs", designH.APIListDesigns)
	mux.HandleFunc("POST /api/designs", designH.APICreateDesign)
	mux.HandleFunc("GET /api/designs/active", designH.APIGetActiveDesign)
	mux.HandleFunc("GET /api/designs/{id}", designH.APIGetDesign)
	mux.HandleFunc("PUT /api/designs/{id}", designH.APIUpdateDesign)
	mux.HandleFunc("PATCH /api/designs/{id}", designH.APIPatchDesign)
	mux.HandleFunc("DELETE /api/designs/{id}", designH.APIDeleteDesign)
	mux.HandleFunc("POST /api/designs/{id}/activate", designH.APIActivateDesign)
	mux.HandleFunc("POST /api/designs/{id}/duplicate", designH.APIDuplicateDesign)
	mux.HandleFunc("GET /api/designs/{id}/history", designH.APIGetHistory)
	mux.HandleFunc("GET /api/designs/{id}/history/{timestamp}", designH.APIGetHistorySnapshot)
	mux.HandleFunc("POST /api/designs/{id}/history/{timestamp}/restore", designH.APIRestoreHistorySnapshot)

	// Media Library API (new)
	mux.HandleFunc("GET /api/media/images", mediaH.APIListImages)
	mux.HandleFunc("GET /api/media/fonts", mediaH.APIListFonts)
	mux.HandleFunc("GET /api/media/images/thumb/{filename}", mediaH.APIGetThumb)
	mux.HandleFunc("POST /api/media/images/upload", mediaH.APIUploadImage)
	mux.HandleFunc("POST /api/media/fonts/upload", mediaH.APIUploadFont)
	mux.HandleFunc("DELETE /api/media/images/{filename}", mediaH.APIDeleteImage)
	mux.HandleFunc("DELETE /api/media/fonts/{filename}", mediaH.APIDeleteFont)

	// Weather endpoints
	mux.HandleFunc("GET /weather_styles", weatherH.ListStyles)
	mux.HandleFunc("GET /location_search", weatherH.LocationSearch)

	// Preview
	mux.HandleFunc("GET /preview", previewH.Preview)
	mux.HandleFunc("POST /api/preview_live", previewH.PreviewLive)

	// Display
	mux.HandleFunc("POST /refresh-display", displayH.RefreshDisplay)

	// Settings
	mux.HandleFunc("GET /settings", settingsH.GetSettings)
	mux.HandleFunc("POST /update_settings", settingsH.UpdateSettings)
	mux.HandleFunc("GET /display_profiles", settingsH.ListDisplayProfiles)

	// Refresh control API
	mux.HandleFunc("POST /api/trigger_refresh", settingsH.TriggerRefresh)
	mux.HandleFunc("GET /api/refresh_status", settingsH.RefreshStatus)
	mux.HandleFunc("POST /api/client_heartbeat", settingsH.ClientHeartbeat)

	// Widget API endpoints
	mux.HandleFunc("GET /api/widgets/weather", widgetH.Weather)
	mux.HandleFunc("GET /api/widgets/forecast", widgetH.Forecast)
	mux.HandleFunc("GET /api/widgets/calendar", widgetH.Calendar)
	mux.HandleFunc("GET /api/widgets/news", widgetH.News)
	mux.HandleFunc("GET /api/widgets/system", widgetH.System)
	mux.HandleFunc("GET /api/widgets/custom", widgetH.Custom)
	mux.HandleFunc("GET /api/widget_layouts/{type}", widgetH.Layouts)

	// Health
	mux.HandleFunc("GET /health", handlers.HealthCheck)

	// Static files
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Middleware chain
	corsMiddleware := middleware.CORS(cfg.CORSAllowedOrigins, cfg.DeploymentMode)
	handler := middleware.Logging(corsMiddleware(mux))

	// Server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "port", cfg.Port, "mode", cfg.DeploymentMode, "data_dir", cfg.DataDir)
		slog.Info("server ready", "url", "http://0.0.0.0:"+cfg.Port+"/designer")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	slog.Info("server shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("server stopped")
}
