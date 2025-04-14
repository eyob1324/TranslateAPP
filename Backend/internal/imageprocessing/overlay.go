package imageprocessing // Changed to main for example usage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/disintegration/imaging"        // For image transformations like rotation
	"github.com/golang/freetype/truetype"      // For font parsing and handling
	"github.com/rwcarlsen/goexif/exif"         // For reading image orientation metadata
	"golang.org/x/image/font"                  // For font rendering interfaces
	"golang.org/x/image/font/gofont/goregular" // Default Go font
	"golang.org/x/image/math/fixed"            // For fixed-point number calculations used in font rendering
)

// TextBlock defines the text content and its bounding box on the original image.
type TextBlock struct {
	OriginalText   string          // Optional: Original text detected by OCR
	TranslatedText string          // The text to overlay onto the image
	Bounds         image.Rectangle // The rectangle where the text should be placed (relative to original image orientation)
}

// OverlayOptions provides configuration for the text overlay process.
type OverlayOptions struct {
	// Font is the default/fallback font used if a specific font for the target language isn't found or specified.
	Font *truetype.Font

	// FontMap holds pre-loaded fonts, keyed by language code (e.g., "ja", "en", "am").
	// This map should be populated by calling utils.LoadAvailableFonts() at application startup.
	FontMap map[string]*truetype.Font

	// FontSize specifies the point size for rendering the text.
	FontSize float64

	// FontColor defines the color of the overlaid text.
	FontColor color.Color

	// BgColor defines the background color of the rectangle drawn behind the text.
	BgColor color.Color

	// OutputFormat specifies the desired image format ("jpeg" or "png"). If empty, uses the original format.
	OutputFormat string

	// JpegQuality sets the quality (0-100) for JPEG output. Only used if OutputFormat is "jpeg".
	JpegQuality int

	// FallbackFont indicates if the currently active 'Font' field is the system default/fallback.
	// This is set dynamically within OverlayTranslatedText based on font selection.
	FallbackFont bool
}

// DefaultOverlayOptions creates a set of default options using the embedded Go font.
func DefaultOverlayOptions() (*OverlayOptions, error) {
	// Parse the embedded Go Regular font data.
	f, err := truetype.Parse(goregular.TTF)
	if err != nil {
		// This should generally not fail unless the embedded font data is corrupted.
		return nil, fmt.Errorf("failed to parse default Go font: %w", err)
	}
	// Return options with defaults set.
	return &OverlayOptions{
		Font:         f,                                          // Use the loaded Go font as the default
		FontMap:      make(map[string]*truetype.Font),            // Initialize FontMap as empty
		FontSize:     10,                                         // Default font size (adjust as needed)
		FontColor:    color.RGBA{R: 0, G: 0, B: 0, A: 255},       // Opaque White text
		BgColor:      color.RGBA{R: 255, G: 255, B: 255, A: 255}, // Semi-transparent black background
		OutputFormat: "png",                                      // Default to PNG for lossless output
		JpegQuality:  100,                                        // High quality for JPEG if chosen
		FallbackFont: true,                                       // Initially, the Font is the fallback Go font
	}, nil
}

// LoadFont loads a TrueType font from a file path.
// Note: This is kept for potential direct loading but primary loading should use utils.LoadAvailableFonts.
func LoadFont(path string) (*truetype.Font, error) {
	fontBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read font file '%s': %w", path, err)
	}
	f, err := truetype.Parse(fontBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse font file '%s': %w", path, err)
	}
	return f, nil
}

// OverlayTranslatedText downloads an image, corrects its orientation based on EXIF data,
// selects the appropriate font based on targetLanguage and available fonts in opts.FontMap,
// and overlays the provided text blocks onto the image.
// Returns the bytes of the processed image, the output format ("png" or "jpeg"), and any error.
func OverlayTranslatedText(ctx context.Context, imageURL string, textBlocks []TextBlock, opts *OverlayOptions, targetLanguage string) ([]byte, string, error) {
	slog.Info("Starting text overlay process", "imageURL", imageURL, "targetLanguage", targetLanguage, "blockCount", len(textBlocks))

	// --- Option Handling & Font Selection ---
	if opts == nil {
		// If no options are provided, create default ones.
		var err error
		opts, err = DefaultOverlayOptions()
		if err != nil {
			return nil, "", fmt.Errorf("failed to get default options: %w", err)
		}
		slog.Warn("Overlay options not provided by caller, using defaults.")
		// Note: Default options have an empty FontMap, so fallback font will likely be used.
	}

	// Ensure a fallback font is always available in opts.Font.
	if opts.Font == nil {
		f, err := truetype.Parse(goregular.TTF)
		if err != nil {
			// This is a critical failure if no fallback can be loaded.
			return nil, "", fmt.Errorf("fallback font missing in options and failed to load default Go font: %w", err)
		}
		opts.Font = f
		opts.FallbackFont = true // Mark that this is the Go default fallback
		slog.Warn("Fallback font was nil in provided options, using Go default font.")
	}
	// Ensure FontMap is not nil to prevent panics.
	if opts.FontMap == nil {
		opts.FontMap = make(map[string]*truetype.Font)
		slog.Warn("opts.FontMap was nil in provided options, initialized to empty map.")
	}

	// --- Select the primary font for this entire operation based on targetLanguage ---
	fontForThisCall := opts.Font // Start assuming fallback font will be used
	isUsingFallback := true      // Track if the selected font is the fallback

	if targetLanguage != "" {
		// Check if a specific font for the target language exists in the map
		if specificFont, ok := opts.FontMap[targetLanguage]; ok && specificFont != nil {
			fontForThisCall = specificFont // Use the specific font
			isUsingFallback = false
			slog.Info("Using specific font loaded for target language", "language", targetLanguage)
		} else {
			// Log a warning if the specific font wasn't found or was nil
			slog.Warn("Font not found or was nil in FontMap for target language, using fallback font.",
				"target_language", targetLanguage,
				"is_fallback_the_go_default", opts.FallbackFont) // Check if the fallback itself is the Go default
			// fontForThisCall remains opts.Font (the fallback)
			isUsingFallback = true
		}
	} else {
		// Log if no target language was provided, indicating fallback usage
		slog.Warn("No targetLanguage specified by caller, using fallback font.",
			"is_fallback_the_go_default", opts.FallbackFont)
		// fontForThisCall remains opts.Font (the fallback)
		isUsingFallback = true
	}

	// Store the originally provided fallback font (just in case, though likely not needed)
	// originalFallbackFont := opts.Font
	// Temporarily set opts.Font to the font chosen for this call (specific or fallback).
	// This allows drawTextBlock to implicitly use the correct font without signature changes.
	opts.Font = fontForThisCall
	// Update the FallbackFont flag in options to reflect the *currently selected* font.
	opts.FallbackFont = isUsingFallback

	// --- Image Download ---
	slog.Debug("Downloading image", "url", imageURL)
	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create HTTP request for image: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download image from %s: %w", imageURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Consider reading response body for more error details if available
		return nil, "", fmt.Errorf("failed to download image: received status code %d", resp.StatusCode)
	}

	// --- Read Image Bytes ---
	imageBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read image response body: %w", err)
	}
	slog.Debug("Image downloaded successfully", "size_bytes", len(imageBytes))

	// --- EXIF Orientation Handling ---
	var orientation int = 1 // Default orientation (normal)
	exifData, err := exif.Decode(bytes.NewReader(imageBytes))
	if err == nil && exifData != nil {
		// Attempt to read the Orientation tag
		tag, tagErr := exifData.Get(exif.Orientation)
		if tagErr == nil {
			// Attempt to get the integer value of the tag
			val, valErr := tag.Int(0)
			if valErr == nil && val >= 1 && val <= 8 { // Ensure orientation is within valid range
				orientation = val
				slog.Debug("EXIF Orientation tag found", "value", orientation)
			} else if valErr != nil {
				slog.Warn("Could not read EXIF orientation value as integer", "error", valErr)
			} else {
				slog.Warn("Invalid EXIF orientation value found", "value", val)
			}
		} else {
			// Orientation tag itself wasn't found
			slog.Debug("EXIF Orientation tag not found in image metadata.")
		}
	} else if err != nil && !errors.Is(err, io.EOF) && err.Error() != "EOF" {
		// Log if decoding failed for reasons other than no EXIF data (EOF is common)
		// The specific error string check for "EOF" might be needed depending on the exif library version
		slog.Warn("Could not decode EXIF data", "error", err)
	} else {
		slog.Debug("No EXIF data found or failed to decode.")
	}

	// --- Image Decoding ---
	img, originalFormat, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode image data: %w", err)
	}
	origBounds := img.Bounds()
	origWidth := origBounds.Dx()
	origHeight := origBounds.Dy()
	slog.Debug("Image decoded successfully", "original_format", originalFormat, "width", origWidth, "height", origHeight)

	// --- Apply Rotation Based on EXIF ---
	var rotatedImg image.Image = img // Start with the original image
	rotationApplied := false
	// Use imaging library for rotation. Note: Its rotation functions are counter-clockwise.
	switch orientation {
	case 3: // 180 degrees
		rotatedImg = imaging.Rotate180(img)
		rotationApplied = true
		slog.Debug("Applying 180 degree rotation based on EXIF")
	case 6: // 90 degrees clockwise == 270 degrees counter-clockwise
		rotatedImg = imaging.Rotate270(img)
		rotationApplied = true
		slog.Debug("Applying 90 degree clockwise rotation (270 CCW) based on EXIF")
	case 8: // 270 degrees clockwise == 90 degrees counter-clockwise
		rotatedImg = imaging.Rotate90(img)
		rotationApplied = true
		slog.Debug("Applying 270 degree clockwise rotation (90 CCW) based on EXIF")
	case 1:
		// No rotation needed
		slog.Debug("Image orientation is normal (1), no rotation needed.")
	default:
		// Log unhandled orientation values but proceed without rotation
		slog.Warn("Unhandled EXIF orientation value, image will not be rotated", "orientation", orientation)
	}
	slog.Debug("rotation Applied", "rotationApplied", rotationApplied)

	// --- Prepare Canvas ---
	// Create a new RGBA image canvas with the dimensions of the (potentially rotated) image.
	finalBounds := rotatedImg.Bounds()
	slog.Debug("Final image dimensions after rotation", "width", finalBounds.Dx(), "height", finalBounds.Dy())
	rgba := image.NewRGBA(finalBounds)
	// Draw the rotated image onto the new canvas.
	draw.Draw(rgba, finalBounds, rotatedImg, image.Point{}, draw.Src)

	// --- Transform Text Block Bounds ---
	// Adjust the coordinates of the text blocks if the image was rotated.

	// --- Draw Text Blocks ---
	slog.Debug("Starting to draw text blocks", "count", len(textBlocks))
	// Iterate through the (potentially transformed) text blocks.
	for i, block := range textBlocks {
		// Basic validation for bounds dimensions.
		if block.Bounds.Dx() <= 0 || block.Bounds.Dy() <= 0 {
			slog.Warn("Skipping text block with invalid bounds (zero or negative size)", "index", i, "text", block.TranslatedText, "bounds", block.Bounds)
			continue
		}
		// Call drawTextBlock, which will use the font set in opts.Font (chosen based on targetLanguage).
		drawTextBlock(rgba, block, opts)
	}
	slog.Debug("Finished drawing text blocks.")

	// Restore the original fallback font in opts if it was modified.
	// This is good practice if the opts object might be reused, although often it's request-scoped.
	// opts.Font = originalFallbackFont // Commented out: If opts is request-scoped, this isn't strictly necessary.

	// --- Encode Result ---
	var buf bytes.Buffer
	// Determine the output format, defaulting to original or PNG.
	outputFormat := opts.OutputFormat
	if outputFormat == "" {
		outputFormat = originalFormat // Use original format if specified, otherwise default below
	}
	if outputFormat != "jpeg" && outputFormat != "png" {
		slog.Warn("Unsupported output format specified or detected, falling back to PNG", "specified_format", outputFormat)
		outputFormat = "png" // Default to PNG if format is invalid or not jpeg/png
	}

	slog.Debug("Encoding final image", "format", outputFormat)
	switch outputFormat {
	case "jpeg":
		// Encode as JPEG with specified quality.
		err = jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: opts.JpegQuality})
		if err != nil {
			return nil, "", fmt.Errorf("failed to encode final image to JPEG: %w", err)
		}
	case "png":
		// Encode as PNG (lossless).
		err = png.Encode(&buf, rgba)
		if err != nil {
			return nil, "", fmt.Errorf("failed to encode final image to PNG: %w", err)
		}
	}

	slog.Info("Image processing and text overlay completed successfully.", "output_format", outputFormat, "output_size_bytes", buf.Len())
	return buf.Bytes(), outputFormat, nil
}

// drawTextBlock draws a single text block onto the image canvas (img).
// It uses the font, size, color, and background color specified in opts.
// Note: It implicitly uses opts.Font, which should have been set correctly
// by the calling function (OverlayTranslatedText) based on the target language.
func drawTextBlock(img *image.RGBA, block TextBlock, opts *OverlayOptions) {
	// Calculate the intersection of the block's bounds and the image bounds.
	// This prevents drawing outside the canvas.
	clippedBounds := block.Bounds.Intersect(img.Bounds())
	if clippedBounds.Empty() {
		// If the bounds are entirely outside the image, log and skip.
		slog.Warn("Text block bounds are entirely outside the image canvas, skipping.", "text", block.TranslatedText, "bounds", block.Bounds, "canvas_bounds", img.Bounds())
		return
	}

	// Draw the background color rectangle within the clipped bounds.
	draw.Draw(img, clippedBounds, &image.Uniform{C: opts.BgColor}, image.Point{}, draw.Over)

	// --- Prepare Font Face ---
	// Ensure a font is actually set in the options.
	if opts.Font == nil {
		slog.Error("Cannot draw text block, opts.Font is nil", "text", block.TranslatedText)
		return // Cannot proceed without a font object.
	}
	// Create a font face with the specified size and hinting.
	face := truetype.NewFace(opts.Font, &truetype.Options{
		Size:    opts.FontSize,    // Point size
		DPI:     72,               // Standard DPI for screen font size calculations
		Hinting: font.HintingFull, // Hinting improves legibility at smaller sizes
	})
	// It's crucial to close the face when done to release resources.
	defer face.Close()

	// --- Calculate Text Position ---
	// Get font metrics (ascent, descent, etc.) from the face.
	metrics := face.Metrics()
	// Convert fixed-point ascent to integer pixels for easier calculation.
	ascentPx := metrics.Ascent.Ceil() // Height above the baseline

	// Calculate a baseline Y position to roughly center the text vertically within the clipped bounds.
	boxHeight := clippedBounds.Dy()
	// Start from top edge, go down half the box height, then go up half the ascent height.
	// This places the baseline such that the ascent part is centered. It's an approximation.
	// A more precise calculation might involve (boxHeight - (ascentPx + descentPx))/2 + ascentPx
	textY := clippedBounds.Min.Y + (boxHeight / 2) + (ascentPx / 2)

	// Set horizontal starting position with some padding from the left edge.
	textX := clippedBounds.Min.X + 4 // Add 4px left padding

	// Create the starting point for drawing (Dot). Y coordinate represents the baseline.
	point := fixed.Point26_6{
		X: fixed.I(textX), // Convert integer X to fixed-point
		Y: fixed.I(textY), // Convert integer Y baseline to fixed-point
	}

	// --- Draw Text ---
	// Create a font drawer.
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(opts.FontColor),
		Face: face,
		Dot:  point,
	}

	// Optional: Check if the text width exceeds the available box width and log a warning.
	textWidth := d.MeasureString(block.TranslatedText)
	availableWidth := clippedBounds.Dx() - 8 // Account for left (4px) and potential right (4px) padding
	if textWidth > fixed.I(availableWidth) {
		slog.Warn("Text may overflow bounding box horizontally",
			"text", block.TranslatedText,
			"text_width_px", textWidth.Ceil(),
			"available_width_px", availableWidth,
			"bounds", clippedBounds)
		// TODO: Consider implementing text wrapping or truncation here if overflow is common/problematic.
	}

	// Draw the actual text string onto the image.
	d.DrawString(block.TranslatedText)

	// Log details about the drawn text for debugging.
	slog.Debug("Drew text string",
		"text", block.TranslatedText,
		"font_size", opts.FontSize,
		"using_fallback_font", opts.FallbackFont, // Log if the font used was the fallback
		"color", fmt.Sprintf("%#v", opts.FontColor),
		"baseline_point", point, // Log the fixed-point baseline position
		"calculated_y_px", textY, // Log the calculated integer baseline Y
		"clipped_bounds", clippedBounds)
}
