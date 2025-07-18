package filer_test

import (
	"bytes"
	"encoding/base64"
	"log"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiscaler/filer-go"
	"github.com/stretchr/testify/assert"
)

var f *filer.Filer

func init() {
	f = filer.NewFiler()
	defer func() {
		_ = f.Close()
	}()
}

func TestOpen_HTTPURL(t *testing.T) {
	err := f.Open("http://examples-1251000004.cos.ap-shanghai.myqcloud.com/sample.jpeg")
	if err != nil {
		log.Panic("get http file failed", err)
	}
	assert.NoError(t, err)

	assert.Equal(t, "sample.jpeg", f.Name())
	assert.Equal(t, ".jpeg", f.Ext())

	size, err := f.Size()
	assert.NoError(t, err)
	assert.Equal(t, int64(158421), size)

	filePath, err := f.SaveTo(`.\tmp/ `)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join("tmp", "sample.jpeg"), filePath)
	assert.Equal(t, "/tmp/sample.jpeg", f.Uri())

	filePath, err = f.SaveTo(`.\tmp/a.jpg `)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join("tmp", "a.jpg"), filePath)
	assert.Equal(t, "/tmp/a.jpg", f.Uri())
}

func TestOpen_Base64Data(t *testing.T) {
	err := f.Open("data:," + base64.StdEncoding.EncodeToString([]byte("Hello, World!")))
	assert.NoError(t, err)
	//defer readCloser.Close()
	//
	//buf := new(bytes.Buffer)
	//_, err = buf.ReadFrom(readCloser)
	//assert.NoError(t, err)
	//assert.Equal(t, "Hello, World!", buf.String())
}

func TestOpen_Base64ImageData(t *testing.T) {
	err := f.Open("data:image/jpeg;base64,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")
	assert.NoError(t, err)

	assert.Equal(t, ".jpeg", f.Ext())

	size, err := f.Size()
	assert.NoError(t, err)
	assert.Equal(t, int64(11876), size)

	assert.NoError(t, err)
	_, err = f.SaveTo(`.\tmp/base64.jpg`)
	assert.Equal(t, "/tmp/base64.jpg", f.Uri())
}

func TestOpen_TextContent(t *testing.T) {
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

func TestOpen_LocalFile(t *testing.T) {
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

func TestOpen_OSFile(t *testing.T) {
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

func TestOpen_MultipartFileHeader(t *testing.T) {
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

	e := f.Ext()
	_ = e
	assert.Equal(t, ".txt", f.Ext())

	size, err := f.Size()
	assert.NoError(t, err)
	assert.Equal(t, int64(13), size)

	_, err = f.SaveTo(`.\tmp/test_new.txt`)
	assert.NoError(t, err)
	assert.Equal(t, "/tmp/test_new.txt", f.Uri())
}

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
			fileBytes, err := os.ReadFile(tt.fields.path)
			if err != nil {
				panic(err)
			}
			_ = f.Open(fileBytes)
			a, err := f.SaveTo("./tmp/test-1.jpg")
			assert.Equal(t, nil, err)
			assert.Equal(t, a, "tmp\\test-1.jpg")
		})
	}
}

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
			_ = f.Open(tt.fields.path)
			assert.Equalf(t, tt.want, f.Title(), "Title()")
		})
	}
}
