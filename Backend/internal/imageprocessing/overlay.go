package imageprocessing

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

type TextBlock struct {
	OriginalText   string
	TranslatedText string
	Bounds         image.Rectangle
}

type Processor struct{}

func NewProcessor() (*Processor, error) {
	return &Processor{}, nil
}

func (p *Processor) OverlayTranslatedText(imageURL string, textBlocks []TextBlock) ([]byte, error) {
	// Download the image
	resp, err := http.Get(imageURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %v", err)
	}
	defer resp.Body.Close()

	// Decode the image
	img, format, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}

	// Create a new RGBA image
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, image.Point{}, draw.Src)

	// Draw each text block
	for _, block := range textBlocks {
		p.drawTextBlock(rgba, block)
	}

	// Encode the result
	var buf bytes.Buffer
	switch format {
	case "jpeg":
		err = jpeg.Encode(&buf, rgba, nil)
	case "png":
		err = png.Encode(&buf, rgba)
	default:
		return nil, fmt.Errorf("unsupported image format: %s", format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to encode image: %v", err)
	}

	return buf.Bytes(), nil
}

func (p *Processor) drawTextBlock(img *image.RGBA, block TextBlock) {
	// Draw semi-transparent background
	draw.Draw(img, block.Bounds,
		&image.Uniform{color.RGBA{0, 0, 0, 128}}, image.Point{}, draw.Over)

	// Draw text
	point := fixed.Point26_6{
		X: fixed.Int26_6(block.Bounds.Min.X << 6),
		Y: fixed.Int26_6((block.Bounds.Min.Y + 12) << 6), // 12 is the font size
	}

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA{255, 255, 255, 255}),
		Face: basicfont.Face7x13,
		Dot:  point,
	}
	d.DrawString(block.TranslatedText)
}
