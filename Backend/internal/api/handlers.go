package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/eyob1324/ocr-translate-Backend/config"
	//"github.com/eyob1324/ocr-translate-Backend/internal/imageprocessing"
	"github.com/eyob1324/ocr-translate-Backend/internal/ocr"
	//"github.com/eyob1324/ocr-translate-Backend/internal/translate"
)

type Handler struct {
	config     *config.Config
	ocrService *ocr.Service
	//translateService *translate.Service
	//imageProcessor   *imageprocessing.Processor
}

func NewHandler(cfg *config.Config) (*Handler, error) {
	ocrService, err := ocr.NewService(cfg.GoogleVisionAPIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCR service: %v", err)
	}
	/*
		translateService, err := translate.NewService(cfg.GoogleTranslateAPIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create translation service: %v", err)
		}

		imageProcessor, err := imageprocessing.NewProcessor()
		if err != nil {
			return nil, fmt.Errorf("failed to create image processor: %v", err)
		}
	*/
	return &Handler{
		config:     cfg,
		ocrService: ocrService,
		//translateService: translateService,
		//imageProcessor:   imageProcessor,
	}, nil
}

func (h *Handler) TranslateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		ImageURL   string `json:"image_url"`
		SourceLang string `json:"source_lang"`
		TargetLang string `json:"target_lang"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate the image URL
	if request.ImageURL == "" {
		log.Print("Error: Empty image URL provided")
		http.Error(w, "Image URL is required", http.StatusBadRequest)
		return
	}

	_, err := url.ParseRequestURI(request.ImageURL)
	if err != nil {
		log.Printf("Error: Invalid image URL provided: %v", err)
		http.Error(w, "Invalid image URL", http.StatusBadRequest)
		return
	}

	log.Printf("Processing image URL: %s", request.ImageURL)
	// TODO: Implement the main logic
	// 1. Extract text from image using OCR
	ocrResult, err := h.ocrService.ExtractText(request.ImageURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("OCR failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("OCR completed successfully. Extracted %d text blocks.", len(ocrResult.Blocks))

	response := map[string]interface{}{
		"status":      "ocr_completed",
		"full_text":   ocrResult.FullText,
		"text_blocks": make([]map[string]interface{}, 0, len(ocrResult.Blocks)),
	}

	for _, block := range ocrResult.Blocks {
		textBlock := map[string]interface{}{
			"text":       block.Text,
			"confidence": block.Confidence,
			"bounds": map[string]int{
				"x1": block.Bounds.Min.X,
				"y1": block.Bounds.Min.Y,
				"x2": block.Bounds.Max.X,
				"y2": block.Bounds.Max.Y,
			},
		}
		response["text_blocks"] = append(response["text_blocks"].([]map[string]interface{}), textBlock)
	}
	// 2. Translate the extracted text
	// 3. Overlay the translated text on the image
	// 4. Return the processed image

	// For now, we'll just return a placeholder response

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
