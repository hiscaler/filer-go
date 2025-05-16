package filer

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"mime/multipart"
)

type Filer struct {
	path        string
	name        string
	readCloser  io.ReadCloser
	writeCloser io.WriteCloser
	error       error
}

func NewFiler() *Filer {
	return &Filer{}
}

func (f *Filer) Open(file any) error {
	switch s := file.(type) {
	case string:
		f.path = s
		if strings.HasPrefix(s, "http") {
			//var resp *http.Response
			if resp, err := http.Get(s); err != nil {
				return err
			} else {
				defer resp.Body.Close()
				var parsedURL *url.URL
				parsedURL, err := url.Parse(f.path)
				if err != nil {
					f.error = err
					return err
				}
				f.name = filepath.Base(parsedURL.Path)
				f.readCloser = resp.Body
			}

		} else if strings.HasPrefix(s, "data:") {
			// 处理 base64 编码的文件
			_, data, found := strings.Cut(s, ",")
			if !found {
				return errors.New("invalid base64 format")
			}

			decodedData, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				return err
			}
			f.readCloser = io.NopCloser(bytes.NewReader(decodedData))
		} else {
			readCloser, err := os.Open(f.path)
			if err != nil {
				return err
			}
			f.name = filepath.Base(f.path)
			defer func(readCloser io.ReadCloser) {
				err = readCloser.Close()
			}(readCloser)
		}
	case *os.File:
		f.path = s.Name()
		f.readCloser = s
	case multipart.File:
		f.readCloser = s
	default:
		return errors.New("unsupported file format")
	}

	return nil
}

func (f *Filer) Name() string {
	return f.name
}

func (f *Filer) Ext() string {
	if f.readCloser == nil {
		return ""
	}

	if seeker, ok := f.readCloser.(io.Seeker); ok {
		// 保存当前位置
		pos, err := seeker.Seek(0, io.SeekCurrent)
		if err == nil {
			var buf [512]byte
			n, err2 := f.readCloser.Read(buf[:])
			// 恢复当前位置
			seeker.Seek(pos, io.SeekStart)
			if err2 == nil || err2 == io.EOF {
				ct := http.DetectContentType(buf[:n])
				switch ct {
				case "image/jpeg":
					return ".jpeg"
				case "image/png":
					return ".png"
				case "image/gif":
					return ".gif"
				}
			}
		}
	}

	return strings.ToLower(path.Ext(f.path))
}

func (f *Filer) Size() (int64, error) {
	if f.readCloser == nil {
		return 0, errors.New("no read file")
	}
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
	return 0, errors.New("readCloser is not a seeker")
}

func (f *Filer) IsEmpty() bool {
	size, err := f.Size()
	return err == nil && size == 0
}

func (f *Filer) IsImage() (bool, error) {
	if f.readCloser == nil {
		return false, errors.New("no read file")
	}
	if seeker, ok := f.readCloser.(io.Seeker); ok {
		// 保存当前位置
		pos, err := seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			return false, err
		}
		// 读取文件内容
		var buf [512]byte
		n, err2 := f.readCloser.Read(buf[:])
		// 恢复当前位置
		seeker.Seek(pos, io.SeekStart)
		if err2 != nil && err2 != io.EOF {
			return false, err2
		}
		// 尝试解码图片
		_, _, err = image.DecodeConfig(bytes.NewReader(buf[:n]))
		if err == nil {
			return true, nil
		}
	}
	return false, nil
}

func (f *Filer) SaveTo(filename string) error {
	if f.readCloser == nil {
		return errors.New("no read file")
	}

	dir := filepath.Dir(filename)
	// Creates dir and subdirectories if they do not exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建 %s 目录失败: %w", dir, err)
	}
	// Creates file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建 %s 文件失败: %w", filename, err)
	}
	defer func() {
		if err1 := file.Close(); err == nil && err1 != nil {
			err = fmt.Errorf("关闭 %s 文件失败: %w", filename, err1)
		}
	}()

	// 写入文件数据
	_, err = io.Copy(file, f.readCloser)
	if err != nil {
		return fmt.Errorf("写入 %s 文件数据失败: %w", filename, err)
	}
	return err
}

func (f *Filer) Close() error {
	if f.readCloser == nil {
		return nil
	}
	return f.readCloser.Close()
}
