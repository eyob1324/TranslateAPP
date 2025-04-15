package utils

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/golang/freetype/truetype" // Import truetype
)

const (
	FontDir = "../../Fonts/"
)

// languageCodes maps language codes (e.g., "en", "ja") to their corresponding font file paths.
// Use language codes consistent with your input (e.g., from DeepL, Google Translate, or HTTP requests).
// Verify that the specified fonts actually support the characters for the assigned languages.
var languageCodes = map[string]string{
	// Latin-based languages using PTSerif
	"en": FontDir + "PTSerif-Regular.ttf",     // English
	"es": FontDir + "PTSerif-Regular.ttf",     // Spanish
	"de": FontDir + "PTSerif-Regular.ttf",     // German
	"it": FontDir + "PTSerif-Regular.ttf",     // Italian
	"pt": FontDir + "PTSerif-Regular.ttf",     // Portuguese (Using "pt" instead of "pt-PT" for simplicity unless distinction is needed)
	"ru": FontDir + "PTSerif-Regular.ttf",     // Russian (Ensure PTSerif includes Cyrillic glyphs)
	"fr": FontDir + "GowunBatang-Regular.ttf", // French (Consider if GowunBatang is the best choice vs a Latin font)

	// East Asian languages
	"zh-CN": FontDir + "ZCOOLXiaoWei-Regular.ttf",   // Chinese (Simplified - using "zh" for simplicity)
	"ja":    FontDir + "MPLUSRounded1c-Regular.ttf", // Japanese
	"ko":    FontDir + "NanumGothic-Regular.ttf",    // Korean
	// Arabic script languages
	"ar": FontDir + "Tajawal-Regular.ttf", // Arabic

	// Ethiopic script languages
	"am": FontDir + "AbyssinicaSIL-Regular.ttf", // Amharic
	"ti": FontDir + "AbyssinicaSIL-Regular.ttf", // Tigrinya

}

// LoadAvailableFonts loads all fonts specified in the languageCodes map.
// It iterates through the map, reads and parses each font file.
// Returns:
//   - map[string]*truetype.Font: A map where keys are language codes and values are the loaded font objects.
//   - error: The first error encountered during loading, or nil if all fonts loaded successfully

func LoadAvailableFonts() (map[string]*truetype.Font, error) {
	loadedFonts := make(map[string]*truetype.Font)
	var firstError error // To report the first issue encountered

	slog.Info("Starting font loading process...")

	for langCode, fontPath := range languageCodes {
		slog.Debug("Attempting to load font", "language", langCode, "path", fontPath)

		// Read the font file bytes
		fontBytes, err := os.ReadFile(fontPath)
		if err != nil {
			slog.Error("Failed to read font file", "language", langCode, "path", fontPath, "error", err)
			// Store the first error encountered
			if firstError == nil {
				firstError = fmt.Errorf("failed to read font '%s' for language '%s': %w", fontPath, langCode, err)
			}
			continue // Attempt to load the next font
		}

		// Parse the font bytes into a truetype.Font object
		f, err := truetype.Parse(fontBytes)
		if err != nil {
			slog.Error("Failed to parse font file", "language", langCode, "path", fontPath, "error", err)
			// Store the first error encountered
			if firstError == nil {
				firstError = fmt.Errorf("failed to parse font '%s' for language '%s': %w", fontPath, langCode, err)
			}
			continue // Attempt to load the next font
		}

		// Successfully loaded and parsed the font
		slog.Debug("Successfully loaded font", "language", langCode, "path", fontPath)
		loadedFonts[langCode] = f
	}

	// Log summary of loading process
	if len(loadedFonts) == 0 && firstError != nil {
		// Critical failure: No fonts loaded at all
		slog.Error("CRITICAL: Failed to load any fonts.", "first_error", firstError)
		// Return nil map and the error
		return nil, fmt.Errorf("no fonts could be loaded: %w", firstError)
	} else if firstError != nil {
		// Partial success: Some fonts loaded, but errors occurred
		slog.Warn("Font loading completed with errors. Some languages may use fallback fonts.",
			"loaded_count", len(loadedFonts),
			"total_specified", len(languageCodes),
			"first_error", firstError)
	} else {
		// Full success
		slog.Info("Successfully loaded all specified fonts.", "count", len(loadedFonts))
	}

	return loadedFonts, firstError
}

// GetFontPath is an optional helper function to retrieve the configured
func GetFontPath(langCode string) (string, bool) {
	path, ok := languageCodes[langCode]
	return path, ok
}
