package usecase

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png" // decoders for provider outputs
	"io"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/be/acs/api/internal/modules/image/domain"
)

// thumbnail decodes a PNG/JPEG/WebP image and returns a JPEG that fits in
// domain.ThumbnailMaxDim (pure Go; no FFmpeg needed for stills).
func thumbnail(r io.Reader) ([]byte, int, int, error) {
	src, _, err := image.Decode(r)
	if err != nil {
		return nil, 0, 0, err
	}
	b := src.Bounds()
	w, h := domain.ThumbSize(b.Dx(), b.Dy(), domain.ThumbnailMaxDim)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, 0, 0, err
	}
	return out.Bytes(), w, h, nil
}
