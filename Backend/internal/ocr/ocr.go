package ocr

import (
	"bytes"
	"context"
	"fmt"
	stdimage "image"
	"io/ioutil"
	"math"
	"net/http"

	vision "cloud.google.com/go/vision/apiv1"
	"google.golang.org/api/option"
	visionpb "google.golang.org/genproto/googleapis/cloud/vision/v1"
)

type TextBlock struct {
	Text       string
	Bounds     stdimage.Rectangle
	Confidence float32
}

type OCRResult struct {
	FullText string
	Blocks   []TextBlock
}

type Service struct {
	client *vision.ImageAnnotatorClient
}

func NewService(apiKey string) (*Service, error) {
	ctx := context.Background()

	client, err := vision.NewImageAnnotatorClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	return &Service{client: client}, nil
}

func (s *Service) ExtractText(imageURL string) (*OCRResult, error) {
	ctx := context.Background()

	// Download the image
	resp, err := http.Get(imageURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %v", err)
	}
	defer resp.Body.Close()

	imageBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image content: %v", err)
	}

	image, err := vision.NewImageFromReader(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create image: %v", err)
	}

	annotation, err := s.client.DetectDocumentText(ctx, image, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to detect text: %v", err)
	}

	if annotation == nil {
		return nil, fmt.Errorf("no text found in the image")
	}

	result := &OCRResult{
		FullText: annotation.Text,
		Blocks:   make([]TextBlock, 0),
	}

	for _, page := range annotation.Pages {
		for _, block := range page.Blocks {
			for _, paragraph := range block.Paragraphs {
				for _, word := range paragraph.Words {
					var wordText string
					for _, symbol := range word.Symbols {
						wordText += symbol.Text
					}

					bounds := calculateBoundingBox(word.BoundingBox.Vertices)

					result.Blocks = append(result.Blocks, TextBlock{
						Text:       wordText,
						Bounds:     bounds,
						Confidence: word.Confidence,
					})
				}
			}
		}
	}

	return result, nil
}

func calculateBoundingBox(vertices []*visionpb.Vertex) stdimage.Rectangle {
	if len(vertices) == 0 {
		return stdimage.Rectangle{}
	}

	minX, minY := math.MaxInt32, math.MaxInt32
	maxX, maxY := math.MinInt32, math.MinInt32

	for _, v := range vertices {
		minX = int(math.Min(float64(minX), float64(v.X)))
		minY = int(math.Min(float64(minY), float64(v.Y)))
		maxX = int(math.Max(float64(maxX), float64(v.X)))
		maxY = int(math.Max(float64(maxY), float64(v.Y)))
	}

	return stdimage.Rect(minX, minY, maxX, maxY)
}

func (s *Service) Close() error {
	return s.client.Close()
}
