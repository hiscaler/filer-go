package filer_test

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiscaler/filer-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

// pngFixture 生成最小合法 PNG，避免依赖仓库外文件。
func pngFixture(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func jpegFixture(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	return buf.Bytes()
}

// openImagerFromPNGFile 通过带 .png 路径打开，保证 Resize/Crop 后 Ext() 仍为 .png
// （Open([]byte) 无 path，流在 EOF 时 sniff 可能误判为文本）。
func openImagerFromPNGFile(t *testing.T, w, h int) *filer.Imager {
	t.Helper()
	data := pngFixture(w, h)
	path := filepath.Join(t.TempDir(), "fixture.png")
	require.NoError(t, os.WriteFile(path, data, 0644))

	f := filer.NewFiler()
	require.NoError(t, f.Open(path))
	t.Cleanup(func() { _ = f.Close() })

	img, err := f.Imager()
	require.NoError(t, err)
	return img
}

func TestImager_DimensionsAndMode(t *testing.T) {
	data := pngFixture(16, 8)
	f := filer.NewFiler()
	require.NoError(t, f.Open(data))

	img, err := f.Imager()
	require.NoError(t, err)

	assert.Equal(t, 16, img.Width())
	assert.Equal(t, 8, img.Height())
	assert.NotEmpty(t, img.Mode())
}

func TestImager_SetQuality(t *testing.T) {
	data := pngFixture(8, 8)
	f := filer.NewFiler()
	require.NoError(t, f.Open(data))
	img, err := f.Imager()
	require.NoError(t, err)

	assert.Equal(t, 100, img.Quality())
	img.SetQuality(85)
	assert.Equal(t, 85, img.Quality())
	img.SetQuality(0)
	assert.Equal(t, 1, img.Quality())
	img.SetQuality(200)
	assert.Equal(t, 100, img.Quality())

	same := img.SetQuality(50)
	assert.Same(t, img, same)
	assert.Equal(t, 50, img.Quality())
}

func TestImager_NotAnImage(t *testing.T) {
	f := filer.NewFiler()
	require.NoError(t, f.Open([]byte("not an image")))

	_, err := f.Imager()
	require.Error(t, err)
}

func TestImager_Body_UntransformedEqualsSource(t *testing.T) {
	data := pngFixture(4, 4)
	f := filer.NewFiler()
	require.NoError(t, f.Open(data))

	img, err := f.Imager()
	require.NoError(t, err)

	out, err := img.Body()
	require.NoError(t, err)
	assert.Equal(t, data, out)
}

func TestImager_FilerBody_Untransformed(t *testing.T) {
	data := pngFixture(4, 4)
	f := filer.NewFiler()
	require.NoError(t, f.Open(data))

	img, err := f.Imager()
	require.NoError(t, err)

	raw, err := img.Filer.Body()
	require.NoError(t, err)
	assert.Equal(t, data, raw)
}

func TestImager_Resize_BodyIsPNG(t *testing.T) {
	img := openImagerFromPNGFile(t, 32, 32)

	require.NoError(t, img.Resize(10, 10))
	assert.Equal(t, 10, img.Width())
	assert.Equal(t, 10, img.Height())
	out, err := img.Body()
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(out, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}))

	decoded, _, err := image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, 10, decoded.Bounds().Dx())
	assert.Equal(t, 10, decoded.Bounds().Dy())
}

func TestImager_Resize_FromPNGBytesWithoutPath_UsesDecodedFormat(t *testing.T) {
	f := filer.NewFiler()
	require.NoError(t, f.Open(pngFixture(32, 32)))
	img, err := f.Imager()
	require.NoError(t, err)

	require.NoError(t, img.Resize(10, 10))
	out, err := img.Body()
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(out, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}))
}

func TestImager_Resize_FromJPEGBytesWithoutPath_UsesDecodedFormat(t *testing.T) {
	f := filer.NewFiler()
	require.NoError(t, f.Open(jpegFixture(32, 32)))
	img, err := f.Imager()
	require.NoError(t, err)

	require.NoError(t, img.Resize(10, 10))
	out, err := img.Body()
	require.NoError(t, err)
	assert.True(t, len(out) > 2)
	assert.Equal(t, byte(0xFF), out[0])
	assert.Equal(t, byte(0xD8), out[1])
}

func TestImager_Crop(t *testing.T) {
	img := openImagerFromPNGFile(t, 40, 30)

	require.NoError(t, img.Crop(12, 12))
	assert.Equal(t, 12, img.Width())
	assert.Equal(t, 12, img.Height())
	out, err := img.Body()
	require.NoError(t, err)
	decoded, _, err := image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, 12, decoded.Bounds().Dx())
	assert.Equal(t, 12, decoded.Bounds().Dy())
}

func TestImager_SaveTo_Untransformed(t *testing.T) {
	data := pngFixture(5, 5)
	f := filer.NewFiler()
	require.NoError(t, f.Open(data))

	img, err := f.Imager()
	require.NoError(t, err)

	dir := t.TempDir()
	path := filepath.Join(dir, "out.png")
	require.NoError(t, img.SaveTo(path))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestImager_SaveTo_AfterResize(t *testing.T) {
	img := openImagerFromPNGFile(t, 20, 20)
	require.NoError(t, img.Resize(8, 8))

	dir := t.TempDir()
	path := filepath.Join(dir, "small.png")
	require.NoError(t, img.SaveTo(path))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	decoded, _, err := image.Decode(bytes.NewReader(got))
	require.NoError(t, err)
	assert.Equal(t, 8, decoded.Bounds().Dx())
}

func TestImager_SaveTo_EmptyPath(t *testing.T) {
	data := pngFixture(2, 2)
	f := filer.NewFiler()
	require.NoError(t, f.Open(data))

	img, err := f.Imager()
	require.NoError(t, err)

	err = img.SaveTo("   ")
	require.Error(t, err)
}

func TestImager_JPEGFixture_EncodePath(t *testing.T) {
	path := filepath.Join("tests", "test.jpg")
	if _, err := os.Stat(path); err != nil {
		t.Skip("tests/test.jpg not present:", err)
	}

	f := filer.NewFiler()
	require.NoError(t, f.Open(path))

	img, err := f.Imager()
	require.NoError(t, err)

	require.NoError(t, img.Resize(50, 50))
	body, err := img.Body()
	require.NoError(t, err)
	assert.True(t, len(body) > 100)
	assert.Equal(t, byte(0xFF), body[0])
	assert.Equal(t, byte(0xD8), body[1])
}

func TestImager_BMP_RoundTrip(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 12, 12))
	path := filepath.Join(t.TempDir(), "x.bmp")
	wf, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, bmp.Encode(wf, m))
	require.NoError(t, wf.Close())

	f := filer.NewFiler()
	require.NoError(t, f.Open(path))
	t.Cleanup(func() { _ = f.Close() })

	img, err := f.Imager()
	require.NoError(t, err)
	require.NoError(t, img.Resize(6, 6))
	out, err := img.Body()
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(out, []byte("BM")))
	decoded, _, err := image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, 6, decoded.Bounds().Dx())
}

func TestImager_TIFF_RoundTrip(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	path := filepath.Join(t.TempDir(), "x.tiff")
	wf, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, tiff.Encode(wf, m, nil))
	require.NoError(t, wf.Close())

	f := filer.NewFiler()
	require.NoError(t, f.Open(path))
	t.Cleanup(func() { _ = f.Close() })

	img, err := f.Imager()
	require.NoError(t, err)
	require.NoError(t, img.Crop(5, 5))
	out, err := img.Body()
	require.NoError(t, err)
	decoded, _, err := image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, 5, decoded.Bounds().Dx())
}
