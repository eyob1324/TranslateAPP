package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/eyob1324/ocr-translate-Backend/config"

	"github.com/eyob1324/ocr-translate-Backend/internal/imageprocessing"
	"github.com/eyob1324/ocr-translate-Backend/internal/ocr"
	"github.com/eyob1324/ocr-translate-Backend/internal/translate"
)

type Handler struct {
	config           *config.Config
	ocrService       *ocr.Service
	translateService *translate.Service
	imageProcessor   *imageprocessing.Processor
}

func NewHandler(cfg *config.Config) (*Handler, error) {
	ocrService, err := ocr.NewService(cfg.GoogleVisionAPIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCR service: %v", err)
	}

	translateService, err := translate.NewService(cfg.GoogleTranslateAPIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create translation service: %v", err)
	}

	imageProcessor, err := imageprocessing.NewProcessor()
	if err != nil {
		return nil, fmt.Errorf("failed to create image processor: %v", err)
	}

	return &Handler{
		config:           cfg,
		ocrService:       ocrService,
		translateService: translateService,
		imageProcessor:   imageProcessor,
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

	/*
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
	*/

	// Prepare texts for batch translation
	textsToTranslate := make([]string, len(ocrResult.Blocks)+1)
	textsToTranslate[0] = ocrResult.FullText
	for i, block := range ocrResult.Blocks {
		textsToTranslate[i+1] = block.Text
	}
	// 2. Translate the extracted text
	//translatedTexts, err := h.translateService.translatedTexts(textsToTranslate, request.SourceLang, request.TargetLang)
	translatedResults, err := h.translateService.TranslateTexts(textsToTranslate, request.SourceLang, request.TargetLang)
	if err != nil {
		log.Printf("Translation failed: %v", err)
		http.Error(w, fmt.Sprintf("Translation failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Translation completed successfully.")

	// 4. Prepare text blocks for image processing
	textBlocks := make([]imageprocessing.TextBlock, len(ocrResult.Blocks))
	for i, block := range ocrResult.Blocks {
		textBlocks[i] = imageprocessing.TextBlock{
			OriginalText:   block.Text,
			TranslatedText: translatedResults[i+1].Translated,
			Bounds:         block.Bounds,
		}
	}

	// 5. Overlay translated text on the image
	processedImageBytes, err := h.imageProcessor.OverlayTranslatedText(request.ImageURL, textBlocks)
	if err != nil {
		log.Printf("Image processing failed: %v", err)
		http.Error(w, fmt.Sprintf("Image processing failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Image processing completed successfully.")

	// 6. Prepare the response
	response := map[string]interface{}{
		"status":               "translation_completed",
		"original_full_text":   ocrResult.FullText,
		"translated_full_text": translatedResults[0].Translated,
		"processed_image":      base64.StdEncoding.EncodeToString(processedImageBytes),
		"text_blocks":          make([]map[string]interface{}, len(ocrResult.Blocks)),
	}

	for i, block := range ocrResult.Blocks {
		textBlock := map[string]interface{}{
			"original_text":   block.Text,
			"translated_text": translatedResults[i+1].Translated,
			"confidence":      block.Confidence,
			"bounds": map[string]int{
				"x1": block.Bounds.Min.X,
				"y1": block.Bounds.Min.Y,
				"x2": block.Bounds.Max.X,
				"y2": block.Bounds.Max.Y,
			},
		}
		response["text_blocks"].([]map[string]interface{})[i] = textBlock
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
		return
	}

	log.Printf("Response sent successfully. Total text blocks: %d", len(response["text_blocks"].([]map[string]interface{})))
	// 3. Overlay the translated text on the image

	// 4. Return the processed image

	// For now, we'll just return a placeholder response

}
