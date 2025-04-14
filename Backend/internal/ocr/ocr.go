package ocr

import (
	"bytes"
	"context"
	"fmt"
	stdimage "image"
	_ "image/jpeg" // Or other supported formats like _ "image/png"
	"io"
	"log/slog"
	"math"
	"net/http"

	vision "cloud.google.com/go/vision/apiv1"
	"github.com/disintegration/imaging"
	"github.com/rwcarlsen/goexif/exif"
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

func NewService(ctx context.Context, apiKey string) (*Service, error) {
	client, err := vision.NewImageAnnotatorClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	return &Service{client: client}, nil
}

func (s *Service) ExtractText(ctx context.Context, imageURL string) (*OCRResult, error) {
	resp, err := http.Get(imageURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %v", err)
	}
	defer resp.Body.Close()

	imageBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image content: %v", err)
	}

	// Create a reader for EXIF decoding
	exifReader := bytes.NewReader(imageBytes)
	exifData, err := exif.Decode(exifReader)
	var orientation int
	if err == nil && exifData != nil {
		tag, err := exifData.Get(exif.Orientation)
		if err == nil {
			// Check the error from Int()
			val, err := tag.Int(0)
			if err == nil {
				orientation = val
				slog.Debug("EXIF Orientation", "orientation", orientation)
			} else {
				// Handle or log the error if orientation value couldn't be read
				slog.Warn("Could not read EXIF orientation value", "error", err)
				// Keep orientation = 0 (default)
			}
		} else {
			// Log that the orientation tag itself wasn't found (optional, less critical)
			// slog.Warn("EXIF Orientation tag not found", "error", err)
		}
	}

	// Create a reader for image decoding
	img, format, err := stdimage.Decode(bytes.NewReader(imageBytes))
	print(format)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}

	// Rotate the image based on EXIF orientation
	var rotatedImg stdimage.Image
	switch orientation {
	case 3:
		rotatedImg = imaging.Rotate180(img)
	case 6:
		rotatedImg = imaging.Rotate270(img)
	case 8:
		rotatedImg = imaging.Rotate90(img)
	default:
		rotatedImg = img
	}

	// Encode the rotated image back to bytes
	var rotatedBuf bytes.Buffer
	// Choose a format explicitly, e.g., JPEG with default quality
	err = imaging.Encode(&rotatedBuf, rotatedImg, imaging.JPEG)
	if err != nil {
		return nil, fmt.Errorf("failed to encode rotated image: %v", err)
	}
	rotatedImageBytes := rotatedBuf.Bytes()

	// Create a reader for the Vision API
	imageForVision, err := vision.NewImageFromReader(bytes.NewReader(rotatedImageBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create vision image: %v", err)
	}

	annotation, err := s.client.DetectDocumentText(ctx, imageForVision, nil)
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
