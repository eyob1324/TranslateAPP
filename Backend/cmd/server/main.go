package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	firebase "firebase.google.com/go/v4"
	"github.com/eyob1324/ocr-translate-Backend/config"
	"github.com/eyob1324/ocr-translate-Backend/internal/api"
	"github.com/eyob1324/ocr-translate-Backend/internal/auth"
	"google.golang.org/api/option"
)

func main() {

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	//fmt.Println(cfg.FirebaseCredentials)
	// Initialize Firebase app
	opt := option.WithCredentialsJSON(cfg.FirebaseCredentials)
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Fatalf("Failed to initialize Firebase app: %v", err)
	}

	// Get Firebase Auth client
	authClient, err := app.Auth(context.Background())
	if err != nil {
		log.Fatalf("Failed to create Firebase Auth client: %v", err)
	}

	// Initialize API handlers
	handler, err := api.NewHandler(cfg)
	if err != nil {
		log.Fatalf("Failed to create handler: %v", err)
	}

	authMiddleware := auth.NewFirebaseAuthMiddleware(authClient)
	// Set up routes
	http.Handle("/translate", authMiddleware.Authenticate(http.HandlerFunc(handler.TranslateHandler)))

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
