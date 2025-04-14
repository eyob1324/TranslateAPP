package translate

import (
	"context" // Ensure context is imported
	"fmt"

	"cloud.google.com/go/translate"
	"golang.org/x/text/language"
	"google.golang.org/api/option"
)

// Service wraps the Google Cloud Translate client.
type Service struct {
	client *translate.Client
}

// TranslationResult holds the details of a single translation operation.
type TranslationResult struct {
	Original   string
	Translated string
	SourceLang string // Detected or specified source language
	TargetLang string
}

// NewService creates a new translation service instance.
// Consider accepting context here if initialization involves network calls or long setup.
func NewService(ctx context.Context, apiKey string) (*Service, error) {
	// Use the passed-in context if needed for client creation, otherwise Background is often fine for setup.
	client, err := translate.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create translate client: %w", err) // Use %w for error wrapping
	}
	return &Service{client: client}, nil
}

// TranslateText translates a single string.
// Accepts context for cancellation/deadlines.
func (s *Service) TranslateText(ctx context.Context, text, sourceLang, targetLang string) (*TranslationResult, error) {
	// Call the batch translation function, passing the context
	results, err := s.TranslateTexts(ctx, []string{text}, sourceLang, targetLang)
	if err != nil {
		return nil, err // Error already wrapped by TranslateTexts
	}
	if len(results) == 0 {
		// This case should ideally not happen if TranslateTexts returns no error and input wasn't empty,
		// but it's safe defensive programming.
		return nil, fmt.Errorf("no translation returned for text: %q", text)
	}
	return results[0], nil
}

// TranslateTexts translates a batch of strings.
// Accepts context for cancellation/deadlines.
func (s *Service) TranslateTexts(ctx context.Context, texts []string, sourceLang, targetLang string) ([]*TranslationResult, error) {
	if len(texts) == 0 {
		return []*TranslationResult{}, nil // Nothing to translate
	}

	// --- Target Language Parsing ---
	target, err := language.Parse(targetLang)
	if err != nil {
		return nil, fmt.Errorf("invalid target language %q: %w", targetLang, err)
	}

	// --- Source Language Parsing (Handle Auto-Detect) ---
	var source language.Tag
	if sourceLang == "" {
		// Use Undetermined tag to signal auto-detection to the API
		source = language.Und // Or language.Undetermined
	} else {
		source, err = language.Parse(sourceLang)
		if err != nil {
			return nil, fmt.Errorf("invalid source language %q: %w", sourceLang, err)
		}
	}

	// --- Prepare Translation Options ---
	opts := &translate.Options{
		Source: source,
		// Format: "text", // Default is text, can specify HTML if needed
	}

	// --- Call Google Translate API ---
	// Use the passed-in context here
	translations, err := s.client.Translate(ctx, texts, target, opts)
	if err != nil {
		// This error might include context deadline exceeded, network issues, API errors etc.
		return nil, fmt.Errorf("google translate API call failed: %w", err)
	}

	// --- Process Results ---
	// Basic validation: check if we got the expected number of results
	if len(translations) != len(texts) {
		return nil, fmt.Errorf("translation result count mismatch: expected %d, got %d", len(texts), len(translations))
	}

	results := make([]*TranslationResult, len(translations))
	for i, t := range translations {
		detectedSource := sourceLang // Assume specified source unless detected
		if sourceLang == "" {
			detectedSource = t.Source.String() // Use the language detected by the API
		}
		results[i] = &TranslationResult{
			Original:   texts[i],
			Translated: t.Text,
			SourceLang: detectedSource, // Store the detected or specified source language
			TargetLang: targetLang,     // Target language remains as requested
		}
	}

	return results, nil
}

// Close releases resources used by the translation client.
func (s *Service) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil // No error if client wasn't initialized
}
