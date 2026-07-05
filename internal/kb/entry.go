package kb

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Entry struct {
	Slug    string
	Title   string
	Tags    []string
	Date    time.Time
	Body    string
	ModTime time.Time
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(title string) string {
	s := strings.ToLower(title)
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func entryPath(kbDir, slug string) string {
	return filepath.Join(kbDir, slug+".md")
}

func Create(kbDir, title string, slug string, tags []string) (*Entry, error) {
	if slug == "" {
		slug = Slugify(title)
	}
	if slug == "" {
		return nil, fmt.Errorf("slug is required for non-ASCII titles (use --slug)")
	}
	path := entryPath(kbDir, slug)
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("entry %q already exists", slug)
	}

	e := &Entry{
		Slug:  slug,
		Title: title,
		Tags:  tags,
		Date:  time.Now(),
	}

	if err := os.MkdirAll(kbDir, 0755); err != nil {
		return nil, err
	}
	return e, os.WriteFile(path, []byte(e.Marshal()), 0644)
}

// Exists reports whether an entry with the given slug is already on disk.
func Exists(kbDir, slug string) bool {
	_, err := os.Stat(entryPath(kbDir, slug))
	return err == nil
}

// Save writes an entry (frontmatter + body) to disk, overwriting any existing
// file for its slug. Used by the web editor in `gkb serve`; the CLI edits the
// raw file via $EDITOR instead.
func Save(kbDir string, e *Entry) error {
	if e.Slug == "" {
		return fmt.Errorf("entry has no slug")
	}
	if err := os.MkdirAll(kbDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(entryPath(kbDir, e.Slug), []byte(e.Marshal()), 0644)
}

func Load(kbDir, slug string) (*Entry, error) {
	path := entryPath(kbDir, slug)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("entry %q not found", slug)
	}
	e, err := parse(slug, string(data))
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(path); err == nil {
		e.ModTime = info.ModTime()
	}
	return e, nil
}

func Delete(kbDir, slug string) error {
	path := entryPath(kbDir, slug)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("entry %q not found", slug)
	}
	return os.Remove(path)
}

func Rename(kbDir, oldSlug, newSlug string) error {
	if newSlug == "" {
		return fmt.Errorf("new slug is required")
	}
	oldPath := entryPath(kbDir, oldSlug)
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return fmt.Errorf("entry %q not found", oldSlug)
	}
	if oldSlug == newSlug {
		return fmt.Errorf("new slug is the same as the old slug")
	}
	newPath := entryPath(kbDir, newSlug)
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("entry %q already exists", newSlug)
	}
	return os.Rename(oldPath, newPath)
}

func List(kbDir string) ([]*Entry, error) {
	files, err := filepath.Glob(filepath.Join(kbDir, "*.md"))
	if err != nil {
		return nil, err
	}
	var entries []*Entry
	for _, f := range files {
		slug := strings.TrimSuffix(filepath.Base(f), ".md")
		e, err := Load(kbDir, slug)
		if err != nil {
			continue
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ModTime.After(entries[j].ModTime) })
	return entries, nil
}

func Search(kbDir, query string, tag string) ([]*Entry, error) {
	all, err := List(kbDir)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var results []*Entry
	for _, e := range all {
		if tag != "" && !e.hasTag(tag) {
			continue
		}
		if q != "" {
			text := strings.ToLower(e.Title + " " + e.Body)
			if !strings.Contains(text, q) {
				continue
			}
		}
		results = append(results, e)
	}
	return results, nil
}

func (e *Entry) hasTag(tag string) bool {
	for _, t := range e.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Marshal serializes the entry to its on-disk form: YAML-ish frontmatter
// followed by the markdown body. An empty body yields just the frontmatter
// (matching the original `add` behavior, where the body was filled in later via
// $EDITOR).
func (e *Entry) Marshal() string {
	tags := ""
	if len(e.Tags) > 0 {
		tags = "tags: " + strings.Join(e.Tags, ", ") + "\n"
	}
	fm := fmt.Sprintf("---\ntitle: %s\ndate: %s\n%s---\n\n",
		e.Title, e.Date.Format("2006-01-02"), tags)
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fm
	}
	return fm + body + "\n"
}

func parse(slug, content string) (*Entry, error) {
	e := &Entry{Slug: slug}
	if !strings.HasPrefix(content, "---") {
		e.Body = content
		return e, nil
	}
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		e.Body = content
		return e, nil
	}
	for _, line := range strings.Split(parts[1], "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "title:") {
			e.Title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
		} else if strings.HasPrefix(line, "date:") {
			d, _ := time.Parse("2006-01-02", strings.TrimSpace(strings.TrimPrefix(line, "date:")))
			e.Date = d
		} else if strings.HasPrefix(line, "tags:") {
			raw := strings.TrimSpace(strings.TrimPrefix(line, "tags:"))
			for _, t := range strings.Split(raw, ",") {
				e.Tags = append(e.Tags, strings.TrimSpace(t))
			}
		}
	}
	e.Body = strings.TrimSpace(parts[2])
	return e, nil
}
