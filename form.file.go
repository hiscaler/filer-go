package filer

import "mime/multipart"

type FormFile struct {
	File   multipart.File
	Header *multipart.FileHeader
}
