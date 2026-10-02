package models

type ClipboardContent struct {
	ContentType ClipboardContentTypes `json:"content_type"`
	Content     []byte                `json:"content"`
}

type ClipboardContentTypes string

const (
	Text  ClipboardContentTypes = "text"
	Image ClipboardContentTypes = "image"
	File  ClipboardContentTypes = "file"
	Other ClipboardContentTypes = "other"
)
