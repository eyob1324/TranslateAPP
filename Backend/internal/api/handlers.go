package api

import (
	"context" // Added for context propagation
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog" // Use slog for structured logging
	"net/http"
	"net/url"
	"strings"
	"time" // Added for logging request duration

	"github.com/eyob1324/ocr-translate-Backend/config"

	// Assuming these packages now have functions accepting context.Context
	"github.com/eyob1324/ocr-translate-Backend/internal/imageprocessing"
	"github.com/eyob1324/ocr-translate-Backend/internal/ocr"
	"github.com/eyob1324/ocr-translate-Backend/internal/translate"
	"github.com/golang/freetype/truetype" // *** Import truetype for font map type ***
)

// Handler holds dependencies for API handlers.
// *** Added fontMap field ***
type Handler struct {
	config           *config.Config
	ocrService       *ocr.Service              // Assuming ocr.Service methods accept context
	translateService *translate.Service        // Assuming translate.Service methods accept context
	fontMap          map[string]*truetype.Font // *** Store the loaded fonts ***
}

// NewHandler creates a new Handler instance with its dependencies.
// *** Modified signature to accept loadedFonts map ***
func NewHandler(cfg *config.Config, loadedFonts map[string]*truetype.Font) (*Handler, error) {
	// Use background context for initial service creation.
	initCtx := context.Background()

	// Pass context and API key to NewService
	ocrService, err := ocr.NewService(initCtx, cfg.GoogleVisionAPIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCR service: %w", err)
	}

	// Pass context and API key to NewService
	translateService, err := translate.NewService(initCtx, cfg.GoogleTranslateAPIKey)
	if err != nil {
		// Clean up already created services on subsequent failure
		if ocrService != nil {
			if cerr := ocrService.Close(); cerr != nil {
				slog.Error("Failed to close OCR service during handler cleanup", "error", cerr)
			}
		}
		return nil, fmt.Errorf("failed to create translation service: %w", err)
	}

	// *** Validate if fonts were passed (optional but good practice) ***
	if loadedFonts == nil {
		slog.Warn("NewHandler received a nil font map. Font overlays will use fallback only.")
		loadedFonts = make(map[string]*truetype.Font) // Ensure it's not nil
	}

	// Create and return the Handler instance, storing dependencies
	return &Handler{
		config:           cfg,
		ocrService:       ocrService,
		translateService: translateService,
		fontMap:          loadedFonts, // *** Store the font map ***
	}, nil
}

// TranslateHandler processes the image translation request.
func (h *Handler) TranslateHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	ctx := r.Context() // Use request context

	requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	logger := slog.With("request_id", requestID)

	logger.Info("Translate request received", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		logger.Warn("Method not allowed", "method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		ImageURL   string `json:"image_url"`
		SourceLang string `json:"source_lang"` // Optional
		TargetLang string `json:"target_lang"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		logger.Error("Failed to decode request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// --- Input Validation ---
	if request.ImageURL == "" {
		logger.Warn("Empty image URL provided")
		http.Error(w, "Image URL is required", http.StatusBadRequest)
		return
	}
	if request.TargetLang == "" {
		logger.Warn("Empty target language provided")
		http.Error(w, "Target language is required", http.StatusBadRequest)
		return
	}
	if _, err := url.ParseRequestURI(request.ImageURL); err != nil {
		logger.Warn("Invalid image URL provided", "url", request.ImageURL, "error", err)
		http.Error(w, "Invalid image URL", http.StatusBadRequest)
		return
	}

	logger.Info("Processing request", "image_url", request.ImageURL, "target_lang", request.TargetLang, "source_lang", request.SourceLang)

	// --- 1. Extract text using OCR ---
	ocrResult, err := h.ocrService.ExtractText(ctx, request.ImageURL)
	if err != nil {
		logger.Error("OCR failed", "image_url", request.ImageURL, "error", err)
		http.Error(w, fmt.Sprintf("OCR failed: %v", err), http.StatusInternalServerError)
		return
	}
	if ocrResult == nil || len(ocrResult.Blocks) == 0 {
		logger.Warn("OCR completed but found no text blocks", "image_url", request.ImageURL)
		// Return success but indicate no text found, maybe? Or keep error?
		// For now, keeping error as client expects translation.
		http.Error(w, "No text found in image", http.StatusNotFound) // Use 404 Not Found
		return
	}
	logger.Info("OCR completed successfully", "block_count", len(ocrResult.Blocks))

	// --- 2. Translate the extracted text ---
	textsToTranslate := make([]string, len(ocrResult.Blocks)+1)
	textsToTranslate[0] = ocrResult.FullText
	for i, block := range ocrResult.Blocks {
		textsToTranslate[i+1] = block.Text
	}

	translatedResults, err := h.translateService.TranslateTexts(ctx, textsToTranslate, request.SourceLang, request.TargetLang)
	if err != nil {
		logger.Error("Translation failed", "error", err)
		http.Error(w, fmt.Sprintf("Translation failed: %v", err), http.StatusInternalServerError)
		return
	}
	if len(translatedResults) != len(textsToTranslate) {
		logger.Error("Translation result count mismatch", "expected", len(textsToTranslate), "got", len(translatedResults))
		http.Error(w, "Translation failed: result count mismatch", http.StatusInternalServerError)
		return
	}
	logger.Info("Translation completed successfully")

	// --- 3. Prepare text blocks for image processing ---
	textBlocksForOverlay := make([]imageprocessing.TextBlock, len(ocrResult.Blocks))
	for i, block := range ocrResult.Blocks {
		// Clean up potential HTML entities from translation result
		cleanTranslatedText := strings.ToValidUTF8(html.UnescapeString(translatedResults[i+1].Translated), "")
		textBlocksForOverlay[i] = imageprocessing.TextBlock{
			OriginalText:   block.Text,
			TranslatedText: cleanTranslatedText,
			Bounds:         block.Bounds,
		}
	}

	// --- 4. Overlay translated text on the image ---
	// *** Prepare Overlay Options using loaded fonts ***
	opts, err := imageprocessing.DefaultOverlayOptions()
	if err != nil {
		// This error is less likely but possible if default font parsing fails
		logger.Error("Failed to get default overlay options", "error", err)
		http.Error(w, "Internal server error during image processing setup", http.StatusInternalServerError)
		return
	}
	// *** Assign the font map from the handler to the options ***
	opts.FontMap = h.fontMap // Use the map stored in the handler

	// *** Call OverlayTranslatedText with the configured 'opts', not nil ***
	processedImageBytes, outputFormat, err := imageprocessing.OverlayTranslatedText(
		ctx,
		request.ImageURL,
		textBlocksForOverlay,
		opts, // Pass the configured options struct
		request.TargetLang,
	)
	if err != nil {
		logger.Error("Image processing (overlay) failed", "error", err)
		http.Error(w, fmt.Sprintf("Image processing failed: %v", err), http.StatusInternalServerError)
		return
	}
	logger.Info("Image processing overlay completed successfully", "output_format", outputFormat)

	// --- 5. Prepare the response ---
	// (Response preparation remains the same)
	response := map[string]interface{}{
		"status":                 "translation_completed",
		"original_full_text":     ocrResult.FullText,
		"translated_full_text":   html.UnescapeString(translatedResults[0].Translated),
		"processed_image":        base64.StdEncoding.EncodeToString(processedImageBytes),
		"processed_image_format": outputFormat,
		"text_blocks":            make([]map[string]interface{}, len(ocrResult.Blocks)),
	}
	for i, block := range ocrResult.Blocks {
		textBlockData := map[string]interface{}{
			"original_text":   block.Text,
			"translated_text": html.UnescapeString(translatedResults[i+1].Translated),
			"confidence":      block.Confidence,
			"bounds": map[string]int{
				"x1": block.Bounds.Min.X, "y1": block.Bounds.Min.Y,
				"x2": block.Bounds.Max.X, "y2": block.Bounds.Max.Y,
			},
		}
		response["text_blocks"].([]map[string]interface{})[i] = textBlockData
	}

	// --- 6. Send Response ---
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Log error, but response headers might have already been sent
		logger.Error("Failed to encode and write response body", "error", err)
		// Cannot send http.Error here as headers are likely written
		return
	}

	duration := time.Since(startTime)
	logger.Info("Response sent successfully", "duration", duration.String(), "block_count", len(ocrResult.Blocks))
}

// Close cleans up resources used by the handler's services.
func (h *Handler) Close() error {
	var errorMessages []string // Collect error messages
	slog.Info("Closing handler services...")

	if h.ocrService != nil {
		slog.Debug("Closing OCR service...")
		if err := h.ocrService.Close(); err != nil {
			slog.Error("Error closing OCR service", "error", err)
			errorMessages = append(errorMessages, fmt.Sprintf("ocr close: %v", err))
		}
	}
	if h.translateService != nil {
		slog.Debug("Closing Translate service...")
		if err := h.translateService.Close(); err != nil {
			slog.Error("Error closing Translate service", "error", err)
			errorMessages = append(errorMessages, fmt.Sprintf("translate close: %v", err))
		}
	}

	if len(errorMessages) > 0 {
		slog.Warn("Completed closing handler services with errors.")
		// Return a combined error message
		return errors.New(strings.Join(errorMessages, "; "))
	}

	slog.Info("Handler services closed successfully.")
	return nil
}
