package filer

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"io"
	"net/http"
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

func (f *Filer) Open(file any) (readCloser io.ReadCloser, err error) {
	switch s := file.(type) {
	case string:
		f.path = s
		if strings.HasPrefix(s, "http") {
			var resp *http.Response
			resp, err = http.Get(s)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			readCloser = resp.Body
		} else if strings.HasPrefix(s, "data:") {
			// 处理 base64 编码的文件
			_, data, found := strings.Cut(s, ",")
			if !found {
				return nil, errors.New("invalid base64 format")
			}
			decodedData, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				return nil, err
			}
			readCloser = io.NopCloser(bytes.NewReader(decodedData))
		} else {
			readCloser, err = os.Open(f.path)
			if err != nil {
				return nil, err
			}
			f.name = filepath.Base(f.path)
			defer func(readCloser io.ReadCloser) {
				err = readCloser.Close()
			}(readCloser)
		}
	case *os.File:
		f.path = s.Name()
		f.readCloser = s
		readCloser = s
	case multipart.File:
		f.readCloser = s
		readCloser = s
	default:
		return nil, errors.New("unsupported file format")
	}
	return readCloser, nil
}

func (f *Filer) Name() string {
	if f.readCloser == nil {
		return ""
	}
	return ""
}

func (f *Filer) Ext() string {
	if f.readCloser != nil {
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

func (f *Filer) SaveTo(path string) error {
	if f.readCloser == nil {
		return errors.New("no read file")
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, f.readCloser)
	return err
}

func (f *Filer) Close() error {
	if f.readCloser == nil {
		return nil
	}
	return f.readCloser.Close()
}
