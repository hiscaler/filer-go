package filer_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiscaler/filer-go"
	"github.com/stretchr/testify/assert"
)

// TestOpen_HTTPURL 验证 HTTP URL 打开、属性读取、保存及图片识别链路。
func TestOpen_HTTPURL(t *testing.T) {
	// 用本地 httptest 避免外网依赖
	img := image.NewNRGBA(image.Rect(0, 0, 16, 8))
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	jpegBytes := buf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpegBytes)
	}))
	defer srv.Close()

	f := filer.NewFiler()
	defer func() { _ = f.Close() }()

	err := f.Open(srv.URL + "/sample.jpeg")
	assert.NoError(t, err)

	assert.Equal(t, "sample.jpeg", f.Name())
	assert.Equal(t, ".jpeg", f.Ext())

	size, err := f.Size()
	assert.NoError(t, err)
	assert.Equal(t, int64(len(jpegBytes)), size)

	assert.True(t, f.IsImage())
	_, err = f.Imager()
	assert.NoError(t, err)

	filePath, err := f.SaveTo(`.\tmp/ `)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join("tmp", "sample.jpeg"), filePath)
	assert.Equal(t, "/tmp/sample.jpeg", f.Uri())

	filePath, err = f.SaveTo(`.\tmp/a.jpg `)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join("tmp", "a.jpg"), filePath)
	assert.Equal(t, "/tmp/a.jpg", f.Uri())
}

// TestOpen_Base64Data 验证普通 data URI（非图片）可被正常打开。
func TestOpen_Base64Data(t *testing.T) {
	f := filer.NewFiler()
	defer func() { _ = f.Close() }()
	err := f.Open("data:," + base64.StdEncoding.EncodeToString([]byte("Hello, World!")))
	assert.NoError(t, err)
}

// TestOpen_Base64ImageData 验证 base64 图片 data URI 的识别、保存与 Imager 创建。
func TestOpen_Base64ImageData(t *testing.T) {
	f := filer.NewFiler()
	defer func() { _ = f.Close() }()
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 16, 8)), &jpeg.Options{Quality: 90})
	dataURI := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	err := f.Open(dataURI)
	assert.NoError(t, err)

	assert.Equal(t, ".jpeg", f.Ext())

	size, err := f.Size()
	assert.NoError(t, err)
	assert.Equal(t, int64(buf.Len()), size)

	_, err = f.SaveTo(`.\tmp/base64.jpg`)
	assert.Equal(t, "/tmp/base64.jpg", f.Uri())

	assert.True(t, f.IsImage())
	_, err = f.Imager()
	assert.NoError(t, err)
}

// TestOpen_TextContent 验证普通文本字符串按文本内容处理。
func TestOpen_TextContent(t *testing.T) {
	f := filer.NewFiler()
	defer func() { _ = f.Close() }()
	textContent := "abcdefg"
	err := f.Open(textContent)
	assert.NoError(t, err)

	assert.Equal(t, ".txt", f.Ext())

	size, err := f.Size()
	assert.NoError(t, err)
	assert.Equal(t, int64(7), size)

	_, err = f.SaveTo(`.\tmp/test.txt`)
	assert.NoError(t, err)
	assert.Equal(t, "/tmp/test.txt", f.Uri())
	b, err := f.Body()
	assert.NoError(t, err)
	textContent2 := string(b)
	assert.Equal(t, textContent, textContent2)
}

// TestOpen_LocalFile 验证本地路径打开、属性读取与保存。
func TestOpen_LocalFile(t *testing.T) {
	f := filer.NewFiler()
	defer func() { _ = f.Close() }()
	err := f.Open("./tests/test.jpg")
	assert.NoError(t, err)

	assert.Equal(t, ".jpg", f.Ext())

	size, err := f.Size()
	assert.NoError(t, err)
	assert.Equal(t, int64(11876), size)

	_, err = f.SaveTo(`.\tmp/test_new.jpg`)
	assert.NoError(t, err)
	assert.Equal(t, "/tmp/test_new.jpg", f.Uri())
}

// TestOpen_OSFile 验证 *os.File 作为输入时的读取与保存行为。
func TestOpen_OSFile(t *testing.T) {
	f := filer.NewFiler()
	defer func() { _ = f.Close() }()
	// 创建一个临时文件用于测试
	file, err := os.Open("./tests/test.jpg")
	assert.NoError(t, err)

	err = f.Open(file)
	assert.NoError(t, err)

	assert.Equal(t, ".jpg", f.Ext())

	size, err := f.Size()
	assert.NoError(t, err)
	assert.Equal(t, int64(11876), size)

	filename := filepath.Join(os.TempDir(), "test_new.jpg")
	_, err = f.SaveTo(filename)
	assert.NoError(t, err)
	defer os.Remove(filename)
	assert.Equal(t, "", f.Uri()) // Bad Uri?
}

// TestOpen_MultipartFileHeader 验证 multipart.FileHeader 输入处理。
func TestOpen_MultipartFileHeader(t *testing.T) {
	f := filer.NewFiler()
	defer func() { _ = f.Close() }()
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="./tests/test.txt"`)
	h.Set("Content-Type", "text/plain")

	// 创建一个 form 文件字段
	part, err := writer.CreatePart(h)
	if err != nil {
		t.Fatalf("CreatePart error: %v", err)
	}
	part.Write([]byte("Hello, world!"))
	writer.Close()

	// 解析 multipart 内容
	reader := multipart.NewReader(&b, writer.Boundary())
	form, err := reader.ReadForm(1024)
	if err != nil {
		t.Fatalf("ReadForm error: %v", err)
	}
	defer form.RemoveAll()

	files := form.File["file"]
	if len(files) == 0 {
		t.Fatalf("No file found in form")
	}

	fileHeader := files[0]
	if fileHeader.Filename != "test.txt" {
		t.Errorf("want filename 'test.txt', got %q", fileHeader.Filename)
	}

	err = f.Open(fileHeader)
	assert.NoError(t, err)

	assert.Equal(t, ".txt", f.Ext())

	size, err := f.Size()
	assert.NoError(t, err)
	assert.Equal(t, int64(13), size)

	_, err = f.SaveTo(`.\tmp/test_new.txt`)
	assert.NoError(t, err)
	assert.Equal(t, "/tmp/test_new.txt", f.Uri())
}

// TestFiler_OpenBytes 验证 []byte 输入后可正常保存。
func TestFiler_OpenBytes(t *testing.T) {
	type fields struct {
		path string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"t1", fields{path: "./tests/test.jpg"}, "test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := filer.NewFiler()
			defer func() { _ = f.Close() }()
			fileBytes, err := os.ReadFile(tt.fields.path)
			if err != nil {
				panic(err)
			}
			_ = f.Open(fileBytes)
			a, err := f.SaveTo("./tmp/test-1.jpg")
			assert.NoError(t, err)
			assert.Equal(t, filepath.Join("tmp", "test-1.jpg"), a)
		})
	}
}

// TestFiler_Title 验证常见场景下 Title() 结果。
func TestFiler_Title(t *testing.T) {
	type fields struct {
		path string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"t1", fields{path: "./tests/test.jpg"}, "test"},
		{"t2", fields{path: "./bad-dir/bad-file.jpg"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := filer.NewFiler()
			defer func() { _ = f.Close() }()
			_ = f.Open(tt.fields.path)
			assert.Equalf(t, tt.want, f.Title(), "Title()")
		})
	}
}

// 仓库内仅存在 tests/test.jpg；在大小写敏感的文件系统上不存在 test.JPG，需在临时目录写入大写扩展名文件再测 Title。
func TestFiler_Title_uppercaseExtension(t *testing.T) {
	src := filepath.Join("tests", "test.jpg")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Skip("need tests/test.jpg:", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.JPG")
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}
	ff := filer.NewFiler()
	defer func() { _ = ff.Close() }()
	if err := ff.Open(path); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "sample", ff.Title())
}

// TestOpen_string_pathLikeButFileMissingFallsBackToText 验证路径形态但不存在时回退为文本。
func TestOpen_string_pathLikeButFileMissingFallsBackToText(t *testing.T) {
	p := filepath.Join(t.TempDir(), "no-such-dir", "a.jpg")
	ff := filer.NewFiler()
	defer func() { _ = ff.Close() }()
	assert.NoError(t, ff.Open(p))
	assert.Equal(t, "", ff.Name())
	b, err := ff.Body()
	assert.NoError(t, err)
	assert.Equal(t, p, string(b))
}

// TestOpen_string_decimalLikeTextNotFilePath 验证类似小数文本不被误判为路径。
func TestOpen_string_decimalLikeTextNotFilePath(t *testing.T) {
	ff := filer.NewFiler()
	defer func() { _ = ff.Close() }()
	s := "version 1.2"
	assert.NoError(t, ff.Open(s))
	assert.Equal(t, "", ff.Name())
	b, err := ff.Body()
	assert.NoError(t, err)
	assert.Equal(t, s, string(b))
}

// TestOpen_string_dotDigitIsTextNotHiddenPath 验证 .2 这类字符串按文本处理。
func TestOpen_string_dotDigitIsTextNotHiddenPath(t *testing.T) {
	ff := filer.NewFiler()
	defer func() { _ = ff.Close() }()
	s := ".2"
	assert.NoError(t, ff.Open(s))
	assert.Equal(t, "", ff.Name())
	b, err := ff.Body()
	assert.NoError(t, err)
	assert.Equal(t, s, string(b))
}

// TestOpen_string_forwardSlashRelativePath 验证带正斜杠的相对路径可正常打开。
func TestOpen_string_forwardSlashRelativePath(t *testing.T) {
	src := filepath.Join("tests", "test.jpg")
	if _, err := os.Stat(src); err != nil {
		t.Skip("need tests/test.jpg:", err)
	}
	ff := filer.NewFiler()
	defer func() { _ = ff.Close() }()
	rel := filepath.ToSlash(filepath.Join(".", "tests", "test.jpg"))
	assert.NoError(t, ff.Open(rel))
	assert.Equal(t, "test.jpg", ff.Name())
}

// TestOpen_string_currentDirPrefix 验证 ./ 前缀相对路径可正常打开。
func TestOpen_string_currentDirPrefix(t *testing.T) {
	src := filepath.Join("tests", "test.jpg")
	if _, err := os.Stat(src); err != nil {
		t.Skip("need tests/test.jpg:", err)
	}
	ff := filer.NewFiler()
	defer func() { _ = ff.Close() }()
	p := "." + string(filepath.Separator) + filepath.Join("tests", "test.jpg")
	assert.NoError(t, ff.Open(p))
	assert.Equal(t, "test.jpg", ff.Name())
}

// TestOpen_string_parentDirRelativePath 验证包含 .. 的相对路径可正常打开。
func TestOpen_string_parentDirRelativePath(t *testing.T) {
	p := filepath.Join("tests", "..", "tests", "test.jpg")
	if _, err := os.Stat(p); err != nil {
		t.Skip("need path:", err)
	}
	ff := filer.NewFiler()
	defer func() { _ = ff.Close() }()
	assert.NoError(t, ff.Open(p))
	assert.Equal(t, "test.jpg", ff.Name())
}

// TestOpen_string_plainFilenameWithExtInTemp 验证临时目录中带扩展名文件名可正常打开。
func TestOpen_string_plainFilenameWithExtInTemp(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("tests", "test.jpg"))
	if err != nil {
		t.Skip("need tests/test.jpg:", err)
	}
	td := t.TempDir()
	path := filepath.Join(td, "photo.jpg")
	assert.NoError(t, os.WriteFile(path, b, 0644))
	ff := filer.NewFiler()
	defer func() { _ = ff.Close() }()
	assert.NoError(t, ff.Open(path))
	assert.Equal(t, "photo.jpg", ff.Name())
}
