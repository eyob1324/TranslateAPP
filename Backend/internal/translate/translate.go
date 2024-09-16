package translate

import (
	"context"
	"fmt"

	"cloud.google.com/go/translate"
	"golang.org/x/text/language"
	"google.golang.org/api/option"
)

type Service struct {
	client *translate.Client
}

type TranslationResult struct {
	Original   string
	Translated string
	SourceLang string
	TargetLang string
}

func NewService(apiKey string) (*Service, error) {
	ctx := context.Background()
	client, err := translate.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create translate client: %v", err)
	}

	return &Service{client: client}, nil
}

func (s *Service) TranslateText(text, sourceLang, targetLang string) (*TranslationResult, error) {
	results, err := s.TranslateTexts([]string{text}, sourceLang, targetLang)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no translation returned")
	}
	return results[0], nil
}

func (s *Service) TranslateTexts(texts []string, sourceLang, targetLang string) ([]*TranslationResult, error) {
	ctx := context.Background()

	source, err := language.Parse(sourceLang)
	if err != nil {
		return nil, fmt.Errorf("invalid source language: %v", err)
	}

	target, err := language.Parse(targetLang)
	if err != nil {
		return nil, fmt.Errorf("invalid target language: %v", err)
	}

	translations, err := s.client.Translate(ctx, texts, target, &translate.Options{
		Source: source,
	})
	if err != nil {
		return nil, fmt.Errorf("translation failed: %v", err)
	}

	results := make([]*TranslationResult, len(translations))
	for i, t := range translations {
		results[i] = &TranslationResult{
			Original:   texts[i],
			Translated: t.Text,
			SourceLang: sourceLang,
			TargetLang: targetLang,
		}
	}

	return results, nil
}

func (s *Service) Close() error {
	return s.client.Close()
}
