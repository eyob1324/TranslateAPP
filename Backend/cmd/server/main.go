package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/eyob1324/ocr-translate-Backend/config"
	"github.com/eyob1324/ocr-translate-Backend/internal/api"
)

func main() {

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	fmt.Println(cfg)

	// Initialize API handlers
	handler, err := api.NewHandler(cfg)
	if err != nil {
		log.Fatalf("Failed to create handler: %v", err)
	}

	// Set up routes
	http.HandleFunc("/translate", handler.TranslateHandler)

	// Start the server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Server starting on port %s...\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

}
