package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"e-ink-picture/server/internal/auth"
	"e-ink-picture/server/internal/config"
	"e-ink-picture/server/internal/handlers"
	"e-ink-picture/server/internal/middleware"
	"e-ink-picture/server/internal/models"
	"e-ink-picture/server/internal/services"
)

// version is stamped at build time via -ldflags "-X main.version=vX.Y.Z"
// (specs/E6.2-release-workflow.md AC1); "dev" identifies unstamped builds.
var version = "dev"

//go:embed static/*
var staticFS embed.FS

//go:embed templates/*
var templateFS embed.FS

// janitorInterval is how often expired sessions and rate-limit windows are
// swept (spec E5.1, Architektur-Richtung 2/9).
const janitorInterval = 10 * time.Minute

// noPasswordWarning is logged on startup and hourly while auth is disabled
// (spec E5.1, Architektur-Richtung 4).
const noPasswordWarning = "authentication disabled — no admin password set, anyone on this network has full access; set one via the web UI or EINK_ADMIN_PASSWORD"

// applyMemoryLimit sets the Go runtime soft memory limit before anything else
// allocates. Precedence (specs/E5.6-render-memory.md AC2): EINK_GOMEMLIMIT >
// native GOMEMLIMIT env (honored by the runtime itself, never overridden) >
// 64 MiB default. "off"/"0" disables the limit entirely.
func applyMemoryLimit() {
	einkVal := os.Getenv("EINK_GOMEMLIMIT")
	decision, err := config.ResolveMemLimit(einkVal, os.Getenv("GOMEMLIMIT"))
	if err != nil {
		slog.Warn("invalid EINK_GOMEMLIMIT, using default",
			"value", einkVal, "default_bytes", int64(config.DefaultMemLimitBytes), "error", err)
	}
	if decision.Apply {
		debug.SetMemoryLimit(decision.Bytes)
		slog.Info("memory limit applied", "limit_bytes", decision.Bytes, "source", decision.Source)
		return
	}
	slog.Info("memory limit not set by server", "source", decision.Source)
}

// application bundles the fully wired HTTP stack (router + middleware chain
// exactly as served in production) with the auth components, so tests can
// exercise the complete request path (specs/E5.1-authentication.md AC1).
type application struct {
	handler  http.Handler
	authMgr  *auth.Manager
	sessions *auth.Store
	limiter  *auth.RateLimiter
}

// newApplication builds services, handlers, routes and the middleware chain
// Logging(CORS(Guard(mux))) and applies the auth bootstrap (EINK_ADMIN_PASSWORD)
// including the startup warnings.
func newApplication(cfg *config.Config) (*application, error) {
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
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	// Create services
	designSvc := services.NewDesignService(cfg.DataDir)
	imageSvc := services.NewImageService(cfg.DataDir)
	weatherSvc := services.NewWeatherService(cfg.WeatherAPIKey, cfg.WeatherLocation, cfg.DataDir)
	settingsSvc := services.NewSettingsService(cfg.DataDir, models.DisplayType(cfg.EInkDisplayType))
	previewSvc := services.NewPreviewService(designSvc, weatherSvc, imageSvc, settingsSvc, cfg.DataDir)
	previewSvc.SetMaxConcurrentRenders(cfg.MaxConcurrentRenders)
	slog.Info("render concurrency limit", "max_concurrent_renders", cfg.MaxConcurrentRenders)

	// Ensure at least one design exists (like Python's ensure_active_design on startup)
	if err := designSvc.EnsureDesignExists(); err != nil {
		return nil, fmt.Errorf("ensure design exists: %w", err)
	}

	// Authentication (specs/E5.1-authentication.md): bcrypt hash in
	// data/auth.json; no hash = auth disabled (upgrade path, no lockout).
	authMgr, err := auth.NewManager(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("init auth manager: %w", err)
	}
	if err := authMgr.Bootstrap(cfg.AdminPassword); err != nil {
		return nil, err
	}
	if !authMgr.PasswordSet() {
		slog.Warn(noPasswordWarning)
	} else if cfg.ClientToken == "" {
		slog.Error("client endpoints require session; e-ink client will fail — set EINK_CLIENT_TOKEN")
	}
	sessions := auth.NewStore()
	limiter := auth.NewRateLimiter()

	// Create handlers
	designH := handlers.NewDesignHandler(designSvc, previewSvc)
	mediaH := handlers.NewMediaHandler(imageSvc)
	weatherH := handlers.NewWeatherHandler(weatherSvc)
	previewH := handlers.NewPreviewHandler(previewSvc, designSvc)
	displayH := handlers.NewDisplayHandler(cfg.EInkClientURL)
	settingsH := handlers.NewSettingsHandler(settingsSvc)
	widgetH := handlers.NewWidgetHandler(weatherSvc)
	authH := handlers.NewAuthHandler(authMgr, sessions, limiter, cfg.CookieSecure)
	setupH := handlers.NewSetupHandler(authMgr, settingsSvc, designSvc, imageSvc)

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

	// Login page (public)
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "login.html", nil); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
		}
	})

	// Auth API
	mux.HandleFunc("POST /api/auth/login", authH.Login)
	mux.HandleFunc("POST /api/auth/logout", authH.Logout)
	mux.HandleFunc("POST /api/auth/setup", authH.Setup)
	mux.HandleFunc("GET /api/auth/status", authH.Status)

	// Setup wizard status (public, specs/E2.3-setup-wizard.md)
	mux.HandleFunc("GET /api/setup/status", setupH.Status)

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
	mux.HandleFunc("GET /health", handlers.HealthCheck(version))

	// Static files
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Favicon (E3.7c): browsers probe /favicon.ico on every page load. Serve
	// the embedded icon directly (public, no session) so the probe returns 200
	// instead of guard 401 (anonymous) or router 404 (logged in). The bytes are
	// read once from the embed and served with image/png.
	favicon, err := staticFS.ReadFile("static/favicon.png")
	if err != nil {
		return nil, fmt.Errorf("read favicon: %w", err)
	}
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=604800")
		w.Write(favicon)
	})

	// Middleware chain: Logging(CORS(Guard(mux))). The guard sits before any
	// router match — unregistered routes are denied by default; OPTIONS
	// preflights are answered by CORS and never reach the guard.
	corsOrigins := middleware.ResolveCORSOrigins(cfg.CORSAllowedOrigins, cfg.DeploymentMode)
	guard := middleware.Guard(middleware.GuardConfig{
		Manager:        authMgr,
		Sessions:       sessions,
		ClientToken:    cfg.ClientToken,
		AllowedOrigins: corsOrigins,
	})
	handler := middleware.Logging(middleware.CORS(corsOrigins)(guard(mux)))

	return &application{
		handler:  handler,
		authMgr:  authMgr,
		sessions: sessions,
		limiter:  limiter,
	}, nil
}

func main() {
	applyMemoryLimit()

	cfg := config.Load()

	app, err := newApplication(cfg)
	if err != nil {
		slog.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}

	stopSessionJanitor := app.sessions.StartJanitor(janitorInterval)
	defer stopSessionJanitor()
	stopLimiterJanitor := app.limiter.StartJanitor(janitorInterval)
	defer stopLimiterJanitor()

	// Hourly reminder while authentication is disabled (spec decision 4).
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if !app.authMgr.PasswordSet() {
				slog.Warn(noPasswordWarning)
			}
		}
	}()

	// Server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      app.handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "version", version, "port", cfg.Port, "mode", cfg.DeploymentMode, "data_dir", cfg.DataDir)
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
