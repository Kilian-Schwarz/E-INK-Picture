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
		cfg.DataDir + "/fonts",
		cfg.DataDir + "/weather_styles",
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
	weatherSvc := services.NewWeatherService(cfg.WeatherAPIKey, cfg.WeatherLocation)
	weatherSvc.SetDataDir(cfg.DataDir)
	previewSvc := services.NewPreviewService(designSvc, weatherSvc, imageSvc, cfg.DataDir)

	// Ensure at least one design exists (like Python's ensure_active_design on startup)
	if err := designSvc.EnsureDesignExists(); err != nil {
		slog.Error("failed to ensure design exists", "error", err)
		os.Exit(1)
	}

	// Create handlers
	designH := handlers.NewDesignHandler(designSvc, previewSvc)
	mediaH := handlers.NewMediaHandler(imageSvc)
	weatherH := handlers.NewWeatherHandler(weatherSvc, cfg.DataDir)
	previewH := handlers.NewPreviewHandler(previewSvc, designSvc)

	// Setup router
	mux := http.NewServeMux()

	// Designer UI
	mux.HandleFunc("GET /designer", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "designer.html", nil); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
		}
	})

	// Design endpoints
	mux.HandleFunc("GET /design", designH.GetActive)
	mux.HandleFunc("GET /designs", designH.List)
	mux.HandleFunc("GET /get_design_by_name", designH.GetByName)
	mux.HandleFunc("POST /set_active_design", designH.SetActive)
	mux.HandleFunc("POST /update_design", designH.Update)
	mux.HandleFunc("POST /clone_design", designH.Clone)
	mux.HandleFunc("POST /delete_design", designH.Delete)

	// Media endpoints
	mux.HandleFunc("POST /upload_image", mediaH.Upload)
	mux.HandleFunc("GET /images_all", mediaH.ListImages)
	mux.HandleFunc("GET /image/{filename}", mediaH.GetImage)
	mux.HandleFunc("POST /delete_image", mediaH.DeleteImage)
	mux.HandleFunc("GET /fonts_all", mediaH.ListFonts)
	mux.HandleFunc("GET /font/{filename}", mediaH.GetFont)

	// Weather endpoints
	mux.HandleFunc("GET /weather_styles", weatherH.ListStyles)
	mux.HandleFunc("GET /location_search", weatherH.LocationSearch)

	// Preview
	mux.HandleFunc("GET /preview", previewH.Preview)

	// Settings
	mux.HandleFunc("POST /update_settings", handlers.UpdateSettings)

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
