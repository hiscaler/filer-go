package filer

import (
	"errors"
	"fmt"
	"github.com/disintegration/imaging"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
)

type Imager struct {
	Filer
	Width   int
	Height  int
	Quality int
	rgba    *image.NRGBA
}

func NewImager(filer *Filer) (*Imager, error) {
	var err error
	imager := &Imager{
		Filer:   *filer,
		Quality: 100,
	}
	if seeker, ok := filer.readCloser.(io.Seeker); ok {
		_, err = seeker.Seek(0, io.SeekStart)
		if err != nil {
			return imager, err
		}
	}

	config, _, err := image.DecodeConfig(filer.readCloser)
	if err != nil {
		return imager, err
	}

	imager.Width = config.Width
	imager.Height = config.Height
	return imager, nil

}

func (img *Imager) Resize(width, height, quality int) error {
	origin, _, err := image.Decode(img.readCloser)
	if err != nil {
		return err
	}
	img.rgba = imaging.Resize(origin, width, height, imaging.Lanczos)
	img.rgba = imaging.Resize(origin, width, height, imaging.Lanczos)
	img.Quality = quality
	return nil
}

func (img *Imager) Crop(width, height int) error {
	origin, _, err := image.Decode(img.readCloser)
	if err != nil {
		return err
	}
	img.rgba = imaging.CropAnchor(origin, width, height, imaging.Center)
	img.rgba = imaging.CropAnchor(origin, width, height, imaging.Center)
	return nil
}

func (img *Imager) SetDPI(dpi float64) error {
	// 设置 DPI 通常需要处理图片的元数据，这里简化处理
	return nil
}

func (img *Imager) Save(path string) error {
	if img.writeCloser == nil {
		return errors.New("no write file")
	}
	if img.rgba == nil {
		return errors.New("RGBA 错误")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		err = f.Close()
	}(f)

	switch img.Ext() {
	case ".png":
		err = png.Encode(f, img.rgba)
	case ".gif":
		err = gif.Encode(f, img.rgba, nil)
	case ".jpg", ".jpeg":
		opt := jpeg.Options{
			Quality: img.Quality,
		}
		err = jpeg.Encode(f, img.rgba, &opt)
	default:
		err = fmt.Errorf("invalid '%s' extension name", img.Ext())
	}

	return err
}
