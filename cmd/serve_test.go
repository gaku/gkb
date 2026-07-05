package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	html := renderMarkdown("![](attachments/my-page--1--image.png)", nil)
	if !strings.Contains(html, `src="/attachments/my-page--1--image.png"`) {
		t.Fatalf("attachment URL was not rewritten: %s", html)
	}
}

func TestExpandWikiLinksResolvesBareSlug(t *testing.T) {
	entries := []*kb.Entry{{Slug: "rome", Title: "ローマ"}}
	html := expandWikiLinks("[[rome]]", entries)
	if html != "[rome](/entry/rome)" {
		t.Fatalf("got %q", html)
	}
}

func TestExpandWikiLinksResolvesByTitle(t *testing.T) {
	entries := []*kb.Entry{{Slug: "rome", Title: "ローマ"}}
	html := expandWikiLinks("[[ローマ]]", entries)
	if html != "[ローマ](/entry/rome)" {
		t.Fatalf("got %q", html)
	}
}

func TestExpandWikiLinksSlugPipeTextStillWorks(t *testing.T) {
	entries := []*kb.Entry{{Slug: "rome", Title: "ローマ"}}
	html := expandWikiLinks("[[rome|ローマの休日のロケ地]]", entries)
	if html != "[ローマの休日のロケ地](/entry/rome)" {
		t.Fatalf("got %q", html)
	}
}

func TestExpandWikiLinksUnknownTargetFallsBackToLiteralSlug(t *testing.T) {
	html := expandWikiLinks("[[not-yet-created]]", nil)
	if html != "[not-yet-created](/entry/not-yet-created)" {
		t.Fatalf("got %q", html)
	}
}

func TestBacklinksFindsReferencingEntries(t *testing.T) {
	entries := []*kb.Entry{
		{Slug: "rome", Title: "ローマ"},
		{Slug: "morini", Title: "Morini", Body: "See [[rome]] and [[ローマ]] again."},
		{Slug: "belgae", Title: "Belgae", Body: "No link here."},
	}
	links := backlinks(entries, "rome")
	if len(links) != 1 || links[0].Slug != "morini" {
		t.Fatalf("got %#v", links)
	}
}

func TestBacklinksExcludesSelfReference(t *testing.T) {
	entries := []*kb.Entry{
		{Slug: "rome", Title: "ローマ", Body: "See also [[rome]]."},
	}
	if links := backlinks(entries, "rome"); len(links) != 0 {
		t.Fatalf("got %#v", links)
	}
}

func TestRenderNewFromWikiLinkPrefillsSlugForAsciiToken(t *testing.T) {
	recorder := httptest.NewRecorder()
	renderNewFromWikiLink(recorder, "labienus", t.TempDir())
	body := recorder.Body.String()
	if !strings.Contains(body, `name="slug" value="labienus"`) {
		t.Fatalf("slug not prefilled: %s", body)
	}
	if !strings.Contains(body, `name="title" value="" `) {
		t.Fatalf("title should be left blank: %s", body)
	}
}

func TestRenderNewFromWikiLinkPrefillsTitleForNonAsciiToken(t *testing.T) {
	recorder := httptest.NewRecorder()
	renderNewFromWikiLink(recorder, "ローマ", t.TempDir())
	body := recorder.Body.String()
	if !strings.Contains(body, `name="title" value="ローマ"`) {
		t.Fatalf("title not prefilled: %s", body)
	}
	if !strings.Contains(body, `name="slug" value="" `) {
		t.Fatalf("slug should be left blank: %s", body)
	}
}

func saveRequest(t *testing.T, form url.Values) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestHandleSaveCreatesEntryWithExplicitSlug(t *testing.T) {
	dir := t.TempDir()
	recorder := httptest.NewRecorder()
	form := url.Values{"mode": {"new"}, "slug": {"rome"}, "title": {"ローマ"}, "body": {"テスト"}}
	handleSave(recorder, saveRequest(t, form), dir)

	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/entry/rome" {
		t.Fatalf("status = %d, location = %q, body = %s", recorder.Code, recorder.Header().Get("Location"), recorder.Body.String())
	}
	e, err := kb.Load(dir, "rome")
	if err != nil {
		t.Fatal(err)
	}
	if e.Title != "ローマ" {
		t.Fatalf("title = %q", e.Title)
	}
}

func TestHandleSaveRejectsInvalidSlug(t *testing.T) {
	dir := t.TempDir()
	recorder := httptest.NewRecorder()
	form := url.Values{"mode": {"new"}, "slug": {"../../etc/passwd"}, "title": {"evil"}, "body": {""}}
	handleSave(recorder, saveRequest(t, form), dir)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "may only contain") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if kb.Exists(dir, "passwd") || kb.Exists(filepath.Dir(dir), "etc/passwd") {
		t.Fatal("entry should not have been written outside kbDir")
	}
}

const sectionTestBody = `# Intro

intro text

## Culture

culture text

### Economy

economy text

## Trade

trade text`

func TestSplitSectionsIncludesNestedSubsections(t *testing.T) {
	secs := splitSections(sectionTestBody)
	if len(secs) != 4 {
		t.Fatalf("got %d sections: %#v", len(secs), secs)
	}
	culture := sectionText(sectionTestBody, secs, 1)
	if !strings.Contains(culture, "## Culture") || !strings.Contains(culture, "### Economy") || !strings.Contains(culture, "economy text") {
		t.Fatalf("Culture section should include its Economy subsection, got %q", culture)
	}
	if strings.Contains(culture, "## Trade") {
		t.Fatalf("Culture section should stop before the next ## heading, got %q", culture)
	}
}

func TestSplitSectionsIgnoresHeadingsInsideFencedCode(t *testing.T) {
	body := "# Real Heading\n\n```markdown\n## Not a real heading\n```\n\ntext"
	secs := splitSections(body)
	if len(secs) != 1 {
		t.Fatalf("got %d sections: %#v", len(secs), secs)
	}
}

func TestReplaceSectionLeavesRestUntouched(t *testing.T) {
	secs := splitSections(sectionTestBody)
	got := replaceSection(sectionTestBody, secs, 1, "## Culture (edited)\n\nnew text")
	if strings.Contains(got, "### Economy") {
		t.Fatalf("subsection should have been replaced along with its parent, got %q", got)
	}
	if !strings.Contains(got, "# Intro") || !strings.Contains(got, "## Trade") || !strings.Contains(got, "trade text") {
		t.Fatalf("sections outside the edited one should be untouched, got %q", got)
	}
	if !strings.Contains(got, "## Culture (edited)") || !strings.Contains(got, "new text") {
		t.Fatalf("edited section content missing, got %q", got)
	}
}

// TestReplaceSectionInsertsBlankLineBeforeNextHeading is a regression test:
// a replacement that doesn't end in a blank line (easy to do — nothing about
// the section editor calls attention to that trailing line) must not get
// glued directly onto the next section's heading.
func TestReplaceSectionInsertsBlankLineBeforeNextHeading(t *testing.T) {
	body := "# a\n\nhello\n\n# b\n\nworld"
	secs := splitSections(body)
	got := replaceSection(body, secs, 0, "# a\n\nnew content")
	want := "# a\n\nnew content\n\n# b\n\nworld"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSectionTextTrimsTrailingSeparatorBlankLine(t *testing.T) {
	body := "# a\n\nhello\n\n# b\n\nworld"
	secs := splitSections(body)
	got := sectionText(body, secs, 0)
	want := "# a\n\nhello"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAddSectionEditLinksIndexMatchesSplitSections(t *testing.T) {
	entries := []*kb.Entry{{Slug: "page", Body: sectionTestBody}}
	html := renderMarkdown(sectionTestBody, entries)
	html = addSectionEditLinks(html, "page")
	secs := splitSections(sectionTestBody)
	for i := range secs {
		want := fmt.Sprintf(`/edit/page?section=%d`, i)
		if !strings.Contains(html, want) {
			t.Fatalf("missing edit link %q in %s", want, html)
		}
	}
	if strings.Count(html, "section-edit") != len(secs) {
		t.Fatalf("expected %d edit links, got html %s", len(secs), html)
	}
}

func TestHandleSaveWithSectionOnlyReplacesThatSection(t *testing.T) {
	dir := t.TempDir()
	if err := kb.Save(dir, &kb.Entry{Slug: "page", Title: "Page", Date: time.Now(), Body: sectionTestBody}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	form := url.Values{
		"mode": {"edit"}, "slug": {"page"}, "title": {"Page"}, "section": {"1"},
		"body": {"## Culture (edited)\n\nnew text"},
	}
	handleSave(recorder, saveRequest(t, form), dir)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	e, err := kb.Load(dir, "page")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(e.Body, "### Economy") {
		t.Fatalf("Economy subsection should have been replaced with its parent, got %q", e.Body)
	}
	if !strings.Contains(e.Body, "# Intro") || !strings.Contains(e.Body, "## Trade") {
		t.Fatalf("other sections should be untouched, got %q", e.Body)
	}
	if !strings.Contains(e.Body, "## Culture (edited)") {
		t.Fatalf("edited section missing, got %q", e.Body)
	}
}

func TestHandleSaveWithStaleSectionIndexReturnsConflict(t *testing.T) {
	dir := t.TempDir()
	if err := kb.Save(dir, &kb.Entry{Slug: "page", Title: "Page", Date: time.Now(), Body: "# Only Heading\n\ntext"}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	form := url.Values{"mode": {"edit"}, "slug": {"page"}, "title": {"Page"}, "section": {"5"}, "body": {"whatever"}}
	handleSave(recorder, saveRequest(t, form), dir)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
