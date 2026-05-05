package filer

import (
	"image"
	"io"

	"github.com/KarpelesLab/gowebp"
)

func encodeWebP(w io.Writer, m image.Image, quality int) error {
	q := float32(quality)
	if q <= 0 {
		q = 75
	}
	if q > 100 {
		q = 100
	}
	return gowebp.Encode(w, m, &gowebp.Options{
		Lossy:   true,
		Quality: q,
		Method:  4,
	})
}
