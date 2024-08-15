package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type Config struct {
	GoogleVisionAPIKey    string `json:"google_vision_api_key"`
	GoogleTranslateAPIKey string `json:"google_translate_api_key"`
	Port                  string `json:"port"`
}

func Load() (*Config, error) {

	envFiles := []string{".env", "../.env", "../../.env"}
	var envLoaded bool
	for _, file := range envFiles {
		if err := godotenv.Load(file); err == nil {
			fmt.Printf("Loaded .env file from: %s\n", file)
			envLoaded = true
			break
		}
	}

	if !envLoaded {
		fmt.Println("Warning: .env file not found. Using environment variables.")
	}

	// Print current working directory and its contents for debugging
	pwd, _ := os.Getwd()
	fmt.Printf("Current working directory: %s\n", pwd)
	files, _ := filepath.Glob("*")
	fmt.Printf("Files in current directory: %v\n", files)

	config := &Config{
		GoogleVisionAPIKey:    os.Getenv("GOOGLE_VISION_API_KEY"),
		GoogleTranslateAPIKey: os.Getenv("GOOGLE_TRANSLATE_API_KEY"),
		Port:                  os.Getenv("PORT"),
	}

	// Set default port if not specified
	if config.Port == "" {
		config.Port = "8080"
	}

	// Validate required fields
	if config.GoogleVisionAPIKey == "" {
		return nil, fmt.Errorf("GOOGLE_VISION_API_KEY is required")
	}
	if config.GoogleTranslateAPIKey == "" {
		return nil, fmt.Errorf("GOOGLE_TRANSLATE_API_KEY is required")
	}

	return config, nil
}

func (c *Config) Save(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(c)
}
