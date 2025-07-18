package filer

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/guregu/null.v4"
)

const (
	base64Pattern  string = "^(?:[A-Za-z0-9+\\/]{4})*(?:[A-Za-z0-9+\\/]{2}==|[A-Za-z0-9+\\/]{3}=|[A-Za-z0-9+\\/]{4})$"
	dataURIPattern        = `^data:(?:[a-zA-Z]+\/[a-zA-Z0-9-.+]+)(?:;charset=[a-zA-Z0-9-]+)?;base64,[A-Za-z0-9+\/]+=*$`
)

// File type
const (
	network       = "network"      // Network
	base64Type    = "base64"       // Base64
	localFilePath = "local-file"   // Local file
	textContent   = "text-content" // Text content
	osFile        = "os-file"      // Opened file handle
	formFile      = "form-file"    // Form file
	fileBytes     = "bytes"        // File bytes
)

var (
	rxBase64  *regexp.Regexp
	rxDataURI *regexp.Regexp
)

var commonMimeTypeExt map[string]string

func init() {
	rxBase64 = regexp.MustCompile(base64Pattern)
	rxDataURI = regexp.MustCompile(dataURIPattern)
	commonMimeTypeExt = map[string]string{
		// 图片
		"image/jpeg":    ".jpeg",
		"image/png":     ".png",
		"image/gif":     ".gif",
		"image/webp":    ".webp",
		"image/bmp":     ".bmp",
		"image/svg+xml": ".svg",
		"image/tiff":    ".tiff",
		"image/x-icon":  ".ico",

		// 文本
		"text/plain":      ".txt",
		"text/html":       ".html",
		"text/css":        ".css",
		"text/javascript": ".js",
		"text/csv":        ".csv",
		"text/xml":        ".xml",

		// 应用
		"application/json":            ".json",
		"application/pdf":             ".pdf",
		"application/zip":             ".zip",
		"application/gzip":            ".gz",
		"application/x-tar":           ".tar",
		"application/rar":             ".rar",
		"application/x-7z-compressed": ".7z",
		"application/msword":          ".doc",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
		"application/vnd.ms-excel": ".xls",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": ".xlsx",
		"application/vnd.mozilla.xul+xml":                                   ".xul",
		"application/x-shockwave-flash":                                     ".swf",
		"application/xhtml+xml":                                             ".xhtml",
		"application/rtf":                                                   ".rtf",

		// 音频
		"audio/mpeg": ".mp3",
		"audio/wav":  ".wav",
		"audio/ogg":  ".ogg",
		"audio/aac":  ".aac",
		"audio/flac": ".flac",

		// 视频
		"video/mp4":       ".mp4",
		"video/webm":      ".webm",
		"video/ogg":       ".ogv",
		"video/quicktime": ".mov",
		"video/x-msvideo": ".avi",
	}
}

type FileInfo struct {
	Path  null.String    // Path
	Type  null.String    // Type
	Name  null.String    // Name with extension
	Title null.String    // Name without extension
	Uri   null.String    // URI
	Size  null.Int       // Size
	Ext   null.String    // Extension
	Body  *io.ReadCloser // Content
}

type Filer struct {
	path        string
	typ         string
	name        string
	size        int64
	possibleExt string
	ext         string
	uri         string
	readCloser  io.ReadCloser
	writeCloser io.WriteCloser
	error       error
}

type ReadSeekCloser struct {
	*bytes.Reader
}

func (r *ReadSeekCloser) Close() error { return nil }

func NewFiler() *Filer {
	return &Filer{}
}

// Open 打开需要处理的文件
// 支持的文件格式为 network, base64, local file, text-content, os.File, FormFile
func (f *Filer) Open(file any) error {
	// Reset file attributes before open
	f.path = ""
	f.typ = ""
	f.name = ""
	f.size = 0
	f.ext = ""
	f.possibleExt = ""
	f.uri = ""
	f.readCloser = nil
	f.writeCloser = nil
	f.error = nil

	switch s := file.(type) {
	case string:
		f.path = s
		var u *url.URL
		u, err := url.Parse(f.path)
		if err == nil && slices.Contains([]string{"http", "https"}, u.Scheme) && u.Host != "" {
			f.typ = network
			var resp *http.Response
			resp, err = http.Get(s)
			if err != nil {
				return fmt.Errorf("filer: %w", err)
			}
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("filer: response status %s", resp.Status)
			}

			f.name = filepath.Base(u.Path)
			f.readCloser = resp.Body
			f.size = resp.ContentLength
		} else if rxDataURI.MatchString(s) {
			// 处理 base64 编码的文件
			f.typ = base64Type
			parts := strings.Split(s, ";")
			if len(parts) != 2 {
				return errors.New("filer: invalid base64 format")
			}

			decodedData, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(parts[1], "base64,"))
			if err != nil {
				return fmt.Errorf("filer: %w", err)
			}
			f.size = int64(len(decodedData))
			f.readCloser = io.NopCloser(bytes.NewReader(decodedData))
			f.ext = detectFileExt(decodedData)
		} else {
			// 判断是普通文本还是文件路径
			if strings.Contains(s, string(os.PathSeparator)) ||
				strings.HasPrefix(s, ".") ||
				filepath.IsAbs(s) ||
				path.Ext(s) != "" {
				f.typ = localFilePath
				readCloser, err := os.Open(f.path)
				if err != nil {
					return fmt.Errorf("filer: %w", err)
				}
				f.possibleExt = path.Ext(s)
				f.readCloser = readCloser
				f.name = filepath.Base(f.path)
			} else {
				f.typ = textContent
				f.size = int64(len(s))
				f.readCloser = &ReadSeekCloser{bytes.NewReader([]byte(s))}
			}
		}
	case []byte:
		f.typ = fileBytes
		f.size = int64(len(s))
		f.readCloser = &ReadSeekCloser{bytes.NewReader(s)}
	case *os.File:
		f.typ = osFile
		f.path = s.Name()
		f.possibleExt = filepath.Ext(s.Name())
		f.readCloser = s
	case multipart.File:
		f.typ = formFile
		f.readCloser = s
	case FormFile:
		f.typ = formFile
		f.ext = path.Ext(s.Header.Filename)
		f.size = s.Header.Size
		f.readCloser = s.File
	case *FormFile:
		f.typ = formFile
		f.ext = path.Ext(s.Header.Filename)
		f.size = s.Header.Size
		f.readCloser = s.File
	case *multipart.FileHeader:
		f.typ = formFile
		f1, err := s.Open()
		if err != nil {
			return fmt.Errorf("filer: %w", err)
		}
		f.name = s.Filename
		f.possibleExt = filepath.Ext(s.Filename)
		f.ext = filepath.Ext(s.Filename)
		f.size = s.Size
		f.readCloser = f1
	case nil:
		return errors.New("filer: open data is nil")
	default:
		return fmt.Errorf("filer: unsupported file format %T", s)
	}

	return nil
}

// Name 文件名（带扩展名）
func (f *Filer) Name() string {
	return f.name
}

// Title 文件标题（不带扩展名）
func (f *Filer) Title() string {
	return strings.ReplaceAll(f.name, f.Ext(), "")
}

func detectFileExt(data []byte, suggestExtensions ...string) string {
	mimeType := http.DetectContentType(data)

	suggestExt := ""
	if len(suggestExtensions) != 0 {
		suggestExt = strings.ToLower(suggestExtensions[0])
	}

	// 优先查手动表
	if ext, ok := commonMimeTypeExt[mimeType]; ok && ext == suggestExt {
		return ext
	}
	extensions, _ := mime.ExtensionsByType(mimeType)
	if len(extensions) == 0 {
		return ""
	}

	if suggestExt != "" && slices.Contains(extensions, suggestExt) {
		return suggestExt
	}

	slices.SortStableFunc(extensions, func(a, b string) int {
		return len(b) - len(a)
	})

	if _, v, ok := strings.Cut(mimeType, "/"); ok {
		suffix := "." + strings.ToLower(v)
		for _, ext := range extensions {
			if ext == suffix {
				return ext
			}
		}
	}
	// 没找到就返回第一个
	return extensions[0]
}

// Ext 文件扩展名
// 注意：该函数总是返回全部小写字母的扩展名，无论原始文件的扩展名是什么
func (f *Filer) Ext() string {
	if f.readCloser == nil {
		return ""
	}

	if f.ext != "" && f.possibleExt == "" {
		return strings.ToLower(f.ext)
	}

	if seeker, ok := f.readCloser.(io.Seeker); ok {
		// 保存当前位置
		pos, err := seeker.Seek(0, io.SeekCurrent)
		if err == nil {
			var buf [512]byte
			n, err2 := f.readCloser.Read(buf[:])
			// 恢复当前位置
			_, _ = seeker.Seek(pos, io.SeekStart)
			if err2 == nil || err2 == io.EOF {
				ext := detectFileExt(buf[:n], f.possibleExt)
				if ext != "" {
					return strings.ToLower(ext)
				}
			}
		}
	}

	return strings.ToLower(filepath.Ext(f.path))
}

func (f *Filer) Size() (int64, error) {
	if f.readCloser == nil {
		return 0, errors.New("filer: no read file")
	}

	switch f.typ {
	case network, base64Type, textContent:
		return f.size, nil

	default:
		if seeker, ok := f.readCloser.(io.Seeker); ok {
			size, err := seeker.Seek(0, io.SeekEnd)
			if err != nil {
				return 0, err
			}
			_, err = seeker.Seek(0, io.SeekStart)
			if err != nil {
				return 0, err
			}
			return size, nil
		}
		return 0, errors.New("filer: readCloser is not a seeker")
	}
}

func (f *Filer) IsEmpty() bool {
	size, err := f.Size()
	return err == nil && size == 0
}

func (f *Filer) IsImage() bool {
	if f.readCloser == nil {
		return false
	}
	if seeker, ok := f.readCloser.(io.Seeker); ok {
		// 保存当前位置
		pos, err := seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			return false
		}
		// 读取文件内容
		var buf [512]byte
		n, err2 := f.readCloser.Read(buf[:])
		// 恢复当前位置
		_, _ = seeker.Seek(pos, io.SeekStart)
		if err2 != nil && err2 != io.EOF {
			return false
		}
		// 尝试解码图片
		_, _, err = image.DecodeConfig(bytes.NewReader(buf[:n]))
		if err == nil {
			return true
		}
	}
	return false
}

func (f *Filer) seekStart() error {
	if f.readCloser == nil {
		return nil
	}
	var err error
	if seeker, ok := f.readCloser.(io.Seeker); ok {
		_, err = seeker.Seek(0, io.SeekStart)
	}
	return err
}

func (f *Filer) Body() ([]byte, error) {
	if f.readCloser == nil {
		return nil, errors.New("filer: no read content")
	}

	err := f.seekStart()
	if err != nil {
		return nil, err
	}

	return io.ReadAll(f.readCloser)
}

// SaveTo 保存文件到指定位置
// 如果只指定路径（以 "/" 或者 "\" 结尾），不指定文件名称，将使用原文件名作为保存后的文件名
func (f *Filer) SaveTo(filename string) (string, error) {
	if f.readCloser == nil {
		return "", errors.New("filer: no read file")
	}

	filename = strings.TrimSpace(filename)
	if filename == "" {
		return "", errors.New("filer: filename is can't empty")
	}

	if strings.HasSuffix(filename, "/") || strings.HasSuffix(filename, "\\") {
		// Append file name
		name := f.Name()
		if name == "" {
			name = fmt.Sprintf("%d%s", time.Now().Nanosecond(), f.Ext())
		}
		filename += string(os.PathSeparator) + name
	}
	filename = filepath.Clean(filename)
	uri := ""
	if !filepath.IsAbs(filename) {
		uri = strings.ReplaceAll(filename, "\\", "/")
		if strings.HasPrefix(uri, ".") {
			uri = uri[1:]
		}
		if !strings.HasPrefix(uri, "/") {
			uri = "/" + uri
		}
	}
	f.uri = uri
	filename = filepath.FromSlash(filename)
	dir := filepath.Dir(filename)
	// Creates dir and subdirectories if they do not exist
	if err := os.MkdirAll(dir, 0666); err != nil {
		return "", fmt.Errorf("filer: make %s directory failed, %w", dir, err)
	}
	// Creates file
	if dir == "." || strings.HasSuffix(filename, dir) {
		// 未提供文件名，则使用随机文件名
		filename = filepath.Join(filename, fmt.Sprintf("%d%s", time.Now().Nanosecond(), f.Ext()))
	}
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("filer: create %s file failed, %w", filename, err)
	}
	defer func() {
		if err1 := file.Close(); err == nil && err1 != nil {
			err = fmt.Errorf("filer: close %s file failed, %w", filename, err1)
		}
	}()

	if err = f.seekStart(); err != nil {
		return "", fmt.Errorf("filer: seek %s file data failed, %w", filename, err)
	}
	_, err = io.Copy(file, f.readCloser)
	if err != nil {
		return "", fmt.Errorf("filer: write %s file data failed, %w", filename, err)
	}
	return filename, nil
}

func (f *Filer) Uri() string {
	return f.uri
}

func (f *Filer) Close() error {
	if f.readCloser == nil {
		return nil
	}
	return f.readCloser.Close()
}
