package kb

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const MaxAttachmentSize = 20 << 20 // 20 MiB

var (
	ErrInvalidAttachmentSlug = errors.New("invalid page slug")
	ErrUnsupportedImage      = errors.New("only JPEG, PNG, GIF, and WebP images are supported")
	ErrAttachmentTooLarge    = errors.New("image exceeds the 20 MiB limit")
	attachmentPart           = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
)

type Attachment struct {
	Name   string `json:"name"`
	Markup string `json:"markup"`
}

// StoreAttachment validates and stores an image using a flat filename tied to
// an existing entry. originalName is used only for the readable filename stem;
// the extension comes from the detected content type.
func StoreAttachment(kbDir, slug, originalName string, src io.Reader) (*Attachment, error) {
	if slug == "" || attachmentPart.ReplaceAllString(slug, "") != slug {
		return nil, ErrInvalidAttachmentSlug
	}
	if _, err := Load(kbDir, slug); err != nil {
		return nil, err
	}

	var sniff [512]byte
	n, err := io.ReadFull(src, sniff[:])
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("read image: %w", err)
	}
	extensions := map[string]string{
		"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp",
	}
	ext, ok := extensions[http.DetectContentType(sniff[:n])]
	if !ok {
		return nil, ErrUnsupportedImage
	}

	stem := strings.TrimSuffix(filepath.Base(originalName), filepath.Ext(originalName))
	stem = strings.Trim(attachmentPart.ReplaceAllString(stem, "-"), "-")
	if stem == "" {
		stem = "image"
	}
	dir := filepath.Join(kbDir, "attachments")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	name := fmt.Sprintf("%s--%d--%s%s", slug, time.Now().UnixNano(), stem, ext)
	path := filepath.Join(dir, name)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, err
	}
	written, copyErr := io.Copy(out, io.LimitReader(io.MultiReader(bytes.NewReader(sniff[:n]), src), MaxAttachmentSize+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || written > MaxAttachmentSize {
		os.Remove(path)
		if written > MaxAttachmentSize {
			return nil, ErrAttachmentTooLarge
		}
		if copyErr != nil {
			return nil, copyErr
		}
		return nil, closeErr
	}

	return &Attachment{Name: name, Markup: "![](attachments/" + name + ")"}, nil
}

// CountAttachments returns the number of attachment files stored across the
// whole knowledge base, regardless of which entry they belong to.
func CountAttachments(kbDir string) int {
	matches, _ := filepath.Glob(filepath.Join(kbDir, "attachments", "*"))
	return len(matches)
}

func ListAttachments(kbDir, slug string) []Attachment {
	if slug == "" {
		return nil
	}
	matches, _ := filepath.Glob(filepath.Join(kbDir, "attachments", slug+"--*"))
	items := make([]Attachment, 0, len(matches))
	for _, path := range matches {
		name := filepath.Base(path)
		items = append(items, Attachment{Name: name, Markup: "![](attachments/" + name + ")"})
	}
	return items
}
