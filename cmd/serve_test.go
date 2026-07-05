package cmd

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gaku/gkb/internal/kb"
)

func uploadRequest(t *testing.T, slug, filename string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("image", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/upload/"+slug, &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestHandleUploadStoresPageAttachment(t *testing.T) {
	dir := t.TempDir()
	if err := kb.Save(dir, &kb.Entry{Slug: "my-page", Title: "My Page", Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG\r\n\x1a\nimage data")
	recorder := httptest.NewRecorder()
	handleUpload(recorder, uploadRequest(t, "my-page", "Screen shot!.png", png), dir)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var got kb.Attachment
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Name, "my-page--") || !strings.HasSuffix(got.Name, "--Screen-shot.png") {
		t.Fatalf("unexpected attachment name %q", got.Name)
	}
	if got.Markup != "![](attachments/"+got.Name+")" {
		t.Fatalf("unexpected markup %q", got.Markup)
	}
	stored, err := os.ReadFile(filepath.Join(dir, "attachments", got.Name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, png) {
		t.Fatalf("stored bytes differ: %q", stored)
	}
	items := kb.ListAttachments(dir, "my-page")
	if len(items) != 1 || items[0].Markup != got.Markup {
		t.Fatalf("listed attachments = %#v", items)
	}
}

func TestHandleUploadRejectsNonImage(t *testing.T) {
	dir := t.TempDir()
	if err := kb.Save(dir, &kb.Entry{Slug: "my-page", Title: "My Page", Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handleUpload(recorder, uploadRequest(t, "my-page", "notes.txt", []byte("not an image")), dir)
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnsupportedMediaType)
	}
}

func TestRenderMarkdownRewritesAttachmentURL(t *testing.T) {
	html := renderMarkdown("![](attachments/my-page--1--image.png)")
	if !strings.Contains(html, `src="/attachments/my-page--1--image.png"`) {
		t.Fatalf("attachment URL was not rewritten: %s", html)
	}
}
