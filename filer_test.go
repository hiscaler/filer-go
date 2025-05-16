package filer_test

import (
	"encoding/base64"
	"errors"
	filer2 "github.com/hiscaler/filer-go"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockMultipartFile 模拟 multipart.File 接口
type MockMultipartFile struct {
	mock.Mock
}

func (m *MockMultipartFile) Read(p []byte) (n int, err error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

func (m *MockMultipartFile) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestOpen_HTTPURL(t *testing.T) {
	filer := filer2.NewFiler()
	err := filer.Open("https://img.kwcdn.com/product/fancy/2e2e0355-20a5-4838-9029-6bbb652ee845.jpg")
	assert.NoError(t, err)

	assert.Equal(t, "2e2e0355-20a5-4838-9029-6bbb652ee845.jpg", filer.Name())
	assert.Equal(t, ".jpg", filer.Ext())

	//defer readCloser.Close()
	//
	//buf := new(bytes.Buffer)
	//_, err = buf.ReadFrom(readCloser)
	//assert.NoError(t, err)
	//assert.Equal(t, "Hello, World!", buf.String())
}

func TestOpen_Base64Data(t *testing.T) {
	filer := filer2.NewFiler()
	err := filer.Open("data:," + base64.StdEncoding.EncodeToString([]byte("Hello, World!")))
	assert.NoError(t, err)
	//defer readCloser.Close()
	//
	//buf := new(bytes.Buffer)
	//_, err = buf.ReadFrom(readCloser)
	//assert.NoError(t, err)
	//assert.Equal(t, "Hello, World!", buf.String())
}

func TestOpen_LocalFile(t *testing.T) {
	filer := filer2.NewFiler()

	// 创建一个临时文件用于测试
	tmpFile, err := os.CreateTemp("", "testfile")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	_, err = tmpFile.WriteString("Hello, World!")
	assert.NoError(t, err)
	tmpFile.Close()

	err = filer.Open(tmpFile.Name())
	assert.NoError(t, err)
	//defer readCloser.Close()
	//
	//buf := new(bytes.Buffer)
	//_, err = buf.ReadFrom(readCloser)
	//assert.NoError(t, err)
	//assert.Equal(t, "Hello, World!", buf.String())
}

func TestOpen_OSFile(t *testing.T) {
	filer := filer2.NewFiler()

	// 创建一个临时文件用于测试
	tmpFile, err := os.CreateTemp("", "testfile")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	_, err = tmpFile.WriteString("Hello, World!")
	assert.NoError(t, err)
	tmpFile.Close()

	osFile, err := os.Open(tmpFile.Name())
	assert.NoError(t, err)
	defer osFile.Close()

	err = filer.Open(osFile)
	assert.NoError(t, err)
	//defer readCloser.Close()
	//
	//buf := new(bytes.Buffer)
	//_, err = buf.ReadFrom(readCloser)
	//assert.NoError(t, err)
	//assert.Equal(t, "Hello, World!", buf.String())
}

func TestOpen_MultipartFile(t *testing.T) {
	filer := filer2.NewFiler()

	mockFile := new(MockMultipartFile)
	mockFile.On("Read", mock.Anything).Return(13, nil).Once().Run(func(args mock.Arguments) {
		copy(args[0].([]byte), "Hello, World!")
	})
	mockFile.On("Close").Return(nil)

	err := filer.Open(mockFile)
	assert.NoError(t, err)
	//defer readCloser.Close()
	//
	//buf := new(bytes.Buffer)
	//_, err = buf.ReadFrom(readCloser)
	//assert.NoError(t, err)
	//assert.Equal(t, "Hello, World!", buf.String())
}

func TestOpen_UnsupportedType(t *testing.T) {
	filer := filer2.NewFiler()

	err := filer.Open(123)
	assert.Error(t, err)
}

// mockTransport 模拟 http.RoundTripper 接口
type mockTransport struct {
	responses map[string]*http.Response
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if resp, ok := m.responses[req.URL.String()]; ok {
		return resp, nil
	}
	return nil, errors.New("not found")
}
