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

// defaultHTTPClient 用于拉取网络文件，避免无限阻塞并统一超时策略。
var defaultHTTPClient = &http.Client{
	Timeout: 60 * time.Second,
}

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
	imager      *Imager
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
	f.imager = nil

	switch s := file.(type) {
	case string:
		f.path = s
		var u *url.URL
		u, err := url.Parse(f.path)
		if err == nil && slices.Contains([]string{"http", "https"}, u.Scheme) && u.Host != "" {
			f.typ = network
			var resp *http.Response
			resp, err = defaultHTTPClient.Get(s)
			if err != nil {
				return fmt.Errorf("filer: %w", err)
			}
			if resp.StatusCode != http.StatusOK {
				_ = resp.Body.Close()
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

			var decodedData []byte
			decodedData, err = base64.StdEncoding.DecodeString(strings.TrimPrefix(parts[1], "base64,"))
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
		fn := strings.TrimSpace(s.Header.Filename)
		f.name = fn
		f.ext = path.Ext(fn)
		f.size = s.Header.Size
		f.readCloser = s.File
	case *FormFile:
		f.typ = formFile
		fn := strings.TrimSpace(s.Header.Filename)
		f.name = fn
		f.ext = path.Ext(fn)
		f.size = s.Header.Size
		f.readCloser = s.File
	case *multipart.FileHeader:
		f.typ = formFile
		f1, err := s.Open()
		if err != nil {
			return fmt.Errorf("filer: %w", err)
		}
		fn := strings.TrimSpace(s.Filename)
		f.name = fn
		f.possibleExt = filepath.Ext(fn)
		f.ext = filepath.Ext(fn)
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
	name := strings.TrimSpace(f.name)
	ext := f.Ext()
	if ext == "" || name == "" {
		return name
	}
	n := len(name)
	if n >= len(ext) && strings.EqualFold(name[n-len(ext):], ext) {
		return name[:n-len(ext)]
	}
	return name
}

// mimeBaseType 获取 mime 的基本类型
func mimeBaseType(mediaType string) string {
	if i := strings.Index(mediaType, ";"); i >= 0 {
		return strings.TrimSpace(mediaType[:i])
	}
	return mediaType
}

// lookupCommonMimeExt 查找 mime 的常见扩展名
func lookupCommonMimeExt(mediaType string) (string, bool) {
	if ext, ok := commonMimeTypeExt[mediaType]; ok {
		return ext, true
	}
	if base := mimeBaseType(mediaType); base != mediaType {
		if ext, ok := commonMimeTypeExt[base]; ok {
			return ext, true
		}
	}
	return "", false
}

// detectFileExt 检测文件扩展名
func detectFileExt(data []byte, suggestExtensions ...string) string {
	mimeType := http.DetectContentType(data)

	suggestExt := ""
	if len(suggestExtensions) != 0 {
		suggestExt = strings.ToLower(suggestExtensions[0])
	}

	// 优先查手动表（支持 text/plain; charset=utf-8 等形式；无 suggestExt 时直接采用表内映射）
	if ext, ok := lookupCommonMimeExt(mimeType); ok {
		if suggestExt == "" || ext == suggestExt {
			return ext
		}
	}
	extensions, _ := mime.ExtensionsByType(mimeType)
	if len(extensions) == 0 {
		if base := mimeBaseType(mimeType); base != mimeType {
			extensions, _ = mime.ExtensionsByType(base)
		}
	}
	if len(extensions) == 0 {
		return ""
	}

	if suggestExt != "" && slices.Contains(extensions, suggestExt) {
		return suggestExt
	}

	slices.SortStableFunc(extensions, func(a, b string) int {
		return len(b) - len(a)
	})

	if _, v, ok := strings.Cut(mimeBaseType(mimeType), "/"); ok {
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
				// 流在 EOF 时 n==0，勿对空片做嗅探，否则易误判成文本等，应回退到 path 上的扩展名
				if n > 0 {
					ext := detectFileExt(buf[:n], f.possibleExt)
					if ext != "" {
						return strings.ToLower(ext)
					}
				}
			}
		}
	}

	return strings.ToLower(filepath.Ext(f.path))
}

// Size 文件大小
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

// IsEmpty 判断文件是否为空
func (f *Filer) IsEmpty() bool {
	size, err := f.Size()
	return err == nil && size == 0
}

// IsImage 判断文件是否为图片
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
		// TIFF 等格式的 DecodeConfig 可能需读取超过 512 字节的 IFD
		buf := make([]byte, 64*1024)
		n, err2 := f.readCloser.Read(buf)
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

// seekStart 恢复文件流到起始位置
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

// Body 获取文件内容
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
	// Windows 风格路径在 Unix 上 "\" 不是分隔符，会导致 ".\\tmp/..." 等异常路径；先统一成 "/" 再交给 FromSlash。
	filename = filepath.FromSlash(strings.ReplaceAll(filename, `\`, `/`))

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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("filer: make %s directory failed, %w", dir, err)
	}
	// Creates file（勿用 dir=="."：当前目录下的 out.txt 等会使 Dir 为 "." 导致误判）
	if strings.HasSuffix(filename, dir) {
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

// Uri 获取文件 URI
func (f *Filer) Uri() string {
	return f.uri
}

// Close 关闭文件流
func (f *Filer) Close() error {
	if f.readCloser == nil {
		return nil
	}
	return f.readCloser.Close()
}

// Imager 获取 Imager 实例
func (f *Filer) Imager() (*Imager, error) {
	if !f.IsImage() {
		return nil, errors.New("filer: not an image")
	}

	imager, err := newImager(f)
	if err != nil {
		return nil, err
	}
	f.imager = imager
	return imager, nil
}
