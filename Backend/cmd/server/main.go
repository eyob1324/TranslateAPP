package main

import (
	"context"
	"errors"   // Added for http.ErrServerClosed
	"log/slog" // Use slog for consistency
	"net/http"
	"os"
	"os/signal" // Added for graceful shutdown
	"syscall"   // Added for graceful shutdown
	"time"      // Added for shutdown timeout

	firebase "firebase.google.com/go/v4"
	"github.com/eyob1324/ocr-translate-Backend/config"
	"github.com/eyob1324/ocr-translate-Backend/internal/api"
	"github.com/eyob1324/ocr-translate-Backend/internal/auth"
	"github.com/eyob1324/ocr-translate-Backend/internal/utils" // Import utils package

	// Import truetype for font map type
	"google.golang.org/api/option"
)

func main() {
	// --- Configuration & Logging ---
	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelDebug) // Set desired log level (Debug, Info, Warn, Error)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}
	slog.Info("Configuration loaded successfully")

	// *** Load Fonts ***
	// Load fonts once at application startup using the function from the utils package.
	slog.Info("Loading application fonts...")
	appFonts, loadErr := utils.LoadAvailableFonts() // Call the font loading function
	if loadErr != nil {
		// Log errors but potentially continue if some fonts loaded or fallback is acceptable.
		slog.Error("Error loading one or more fonts, overlay might use fallbacks", "error", loadErr)
		// If *no* fonts loaded and they are essential for operation, exit.
		if appFonts == nil || len(appFonts) == 0 {
			slog.Error("CRITICAL: No fonts were loaded successfully. Exiting.")
			os.Exit(1)
		}
	}
	slog.Info("Font loading complete.", "loaded_count", len(appFonts))
	// The 'appFonts' variable (map[string]*truetype.Font) now holds the loaded fonts.

	// --- Firebase Initialization ---
	initCtx := context.Background()
	opt := option.WithCredentialsJSON(cfg.FirebaseCredentials)
	app, err := firebase.NewApp(initCtx, nil, opt)
	if err != nil {
		slog.Error("Failed to initialize Firebase app", "error", err)
		os.Exit(1)
	}
	slog.Info("Firebase app initialized")

	authClient, err := app.Auth(initCtx)
	if err != nil {
		slog.Error("Failed to create Firebase Auth client", "error", err)
		os.Exit(1)
	}
	slog.Info("Firebase Auth client created")

	handler, err := api.NewHandler(cfg, appFonts) // Pass loaded fonts here
	if err != nil {
		slog.Error("Failed to create API handler", "error", err)
		os.Exit(1)
	}
	slog.Info("API handler initialized")

	// --- Middleware ---
	authMiddleware := auth.NewFirebaseAuthMiddleware(authClient)
	slog.Info("Authentication middleware initialized")

	// --- HTTP Server Setup ---
	mux := http.NewServeMux()
	authenticatedTranslateHandler := authMiddleware.Authenticate(http.HandlerFunc(handler.TranslateHandler))
	mux.Handle("/translate", authenticatedTranslateHandler)
	// Add health check or other routes if needed
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK")) // Use Write instead of Fprintln for simple response
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default port if not set by environment
	}
	addr := ":" + port

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second, // Example timeouts
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// --- Graceful Shutdown Setup ---
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)
	serverStopped := make(chan struct{})

	// Goroutine to run the server
	go func() {
		defer close(serverStopped) // Ensure serverStopped is closed when this goroutine exits
		slog.Info("Server starting", "address", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server failed to start or crashed", "error", err)
			// Optionally signal shutdown immediately if server fails to start
			// shutdownChan <- syscall.SIGTERM
		}
	}()

	// --- Wait for Shutdown Signal ---
	sig := <-shutdownChan
	slog.Info("Shutdown signal received", "signal", sig.String())

	// --- Perform Graceful Shutdown ---
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	slog.Info("Attempting graceful server shutdown...")
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server shutdown failed", "error", err)
	} else {
		slog.Info("Server shutdown completed gracefully")
	}

	slog.Info("Cleanup finished, exiting application.")

	// Wait for the server goroutine to fully stop
	<-serverStopped
	slog.Info("Server goroutine stopped.")
}
