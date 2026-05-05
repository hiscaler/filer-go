package filer

// 注册 BMP、TIFF、WebP 解码器，供 image.Decode、DecodeConfig、Filer.IsImage 等使用。
// WebP 使用 gowebp（纯 Go，无需 CGO；内部仍依赖 x/image/webp 解析部分比特流）。
import (
	_ "github.com/KarpelesLab/gowebp"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
)
