package cmd

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gaku/gkb/internal/kb"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/spf13/cobra"
)

var (
	servePort int
	serveBind string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start a local web server to browse the knowledge base",
	RunE: func(cmd *cobra.Command, args []string) error {
		kbDir := requireKbDir()

		if entries, err := kb.List(kbDir); err == nil {
			warnDuplicateTitles(entries)
		}

		mux := http.NewServeMux()
		attachmentsDir := filepath.Join(kbDir, "attachments")
		mux.Handle("/attachments/", http.StripPrefix("/attachments/", http.FileServer(http.Dir(attachmentsDir))))

		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query().Get("q")
			tag := r.URL.Query().Get("tag")

			var entries []*kb.Entry
			var err error
			if q != "" || tag != "" {
				entries, err = kb.Search(kbDir, q, tag)
			} else {
				entries, err = kb.List(kbDir)
			}
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}

			// Stats reflect the whole knowledge base, not just the filtered
			// results, so they still read as "5 pages" while a search narrows
			// the list down to 1.
			totalEntries := len(entries)
			if q != "" || tag != "" {
				all, err := kb.List(kbDir)
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				totalEntries = len(all)
			}

			renderList(w, entries, q, tag, totalEntries, kb.CountAttachments(kbDir))
		})

		mux.HandleFunc("/entry/", func(w http.ResponseWriter, r *http.Request) {
			slug := strings.TrimPrefix(r.URL.Path, "/entry/")
			// Markdown files link to each other with .md suffixes (e.g.
			// [foo](foo.md)); redirect those to the canonical extension-less URL.
			if strings.HasSuffix(slug, ".md") {
				http.Redirect(w, r, "/entry/"+strings.TrimSuffix(slug, ".md"), http.StatusMovedPermanently)
				return
			}
			e, err := kb.Load(kbDir, slug)
			if err != nil {
				renderNewFromWikiLink(w, slug, kbDir)
				return
			}
			renderEntry(w, e, kbDir)
		})

		mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
			renderEdit(w, &kb.Entry{}, true, "", kbDir)
		})

		mux.HandleFunc("/edit/", func(w http.ResponseWriter, r *http.Request) {
			slug := strings.TrimPrefix(r.URL.Path, "/edit/")
			e, err := kb.Load(kbDir, slug)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if raw := r.URL.Query().Get("section"); raw != "" {
				secs := splitSections(e.Body)
				if idx, err := strconv.Atoi(raw); err == nil && idx >= 0 && idx < len(secs) {
					renderEditSection(w, e, secs, idx, "", kbDir)
					return
				}
			}
			renderEdit(w, e, false, "", kbDir)
		})

		mux.HandleFunc("/upload/", func(w http.ResponseWriter, r *http.Request) {
			handleUpload(w, r, kbDir)
		})

		mux.HandleFunc("/save", func(w http.ResponseWriter, r *http.Request) {
			handleSave(w, r, kbDir)
		})

		mux.HandleFunc("/login", handleLogin)
		mux.HandleFunc("/logout", handleLogout)

		handler := withAuth(mux)

		addr := fmt.Sprintf("%s:%d", serveBind, servePort)
		printListenAddrs()
		return http.ListenAndServe(addr, handler)
	},
}

const sessionCookie = "gkb_session"

// sessionTTL is how long a login stays valid. Cookies survive server restarts
// (the signing key is derived from serve_pass), so this bounds re-logins even
// while the process is bounced by `make install`.
const sessionTTL = 30 * 24 * time.Hour

// withAuth gates the whole site behind a session cookie when serve_user/serve_pass
// are configured. Unauthenticated requests are redirected to a real HTML login
// form (/login) — unlike Basic Auth, that lets password managers save and fill
// the credentials. /login and /logout are always reachable. If credentials are
// unset, the server runs open (a warning is printed at startup).
func withAuth(next http.Handler) http.Handler {
	if cfg.ServeUser == "" || cfg.ServePass == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" || r.URL.Path == "/logout" || validSession(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
	})
}

// sessionKey is the HMAC key for signing session cookies. Deriving it from the
// password keeps the server stateless (no key to persist) while invalidating all
// sessions whenever the password changes.
func sessionKey() []byte { return []byte("gkb-session-v1:" + cfg.ServePass) }

// issueSession signs an expiry timestamp and sets it as the session cookie. The
// value is `<exp-unix>.<hex hmac>`, verified in validSession.
func issueSession(w http.ResponseWriter) {
	exp := time.Now().Add(sessionTTL)
	payload := strconv.FormatInt(exp.Unix(), 10)
	mac := hmac.New(sha256.New, sessionKey())
	mac.Write([]byte(payload))
	value := payload + "." + hex.EncodeToString(mac.Sum(nil))
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		// SameSite=Lax blocks the cookie on cross-site POSTs (basic CSRF defense)
		// while keeping normal navigation working. Secure is omitted so plain-HTTP
		// localhost testing works; behind a TLS proxy the browser<->proxy hop is
		// encrypted regardless.
		SameSite: http.SameSiteLaxMode,
	})
}

// validSession reports whether the request carries a well-signed, unexpired
// session cookie.
func validSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	payload, sig, ok := strings.Cut(c.Value, ".")
	if !ok {
		return false
	}
	mac := hmac.New(sha256.New, sessionKey())
	mac.Write([]byte(payload))
	want := mac.Sum(nil)
	got, err := hex.DecodeString(sig)
	if err != nil || !hmac.Equal(got, want) {
		return false
	}
	exp, err := strconv.ParseInt(payload, 10, 64)
	if err != nil || time.Now().Unix() >= exp {
		return false
	}
	return true
}

// handleLogin renders the login form (GET) and validates credentials (POST). On
// success it issues a session cookie and redirects to the `next` URL.
func handleLogin(w http.ResponseWriter, r *http.Request) {
	next := sanitizeNext(r.FormValue("next"))
	if r.Method == http.MethodPost {
		u := r.FormValue("username")
		p := r.FormValue("password")
		userOK := subtle.ConstantTimeCompare([]byte(u), []byte(cfg.ServeUser)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(p), []byte(cfg.ServePass)) == 1
		if userOK && passOK {
			issueSession(w)
			http.Redirect(w, r, next, http.StatusSeeOther)
			return
		}
		renderLogin(w, next, "invalid username or password")
		return
	}
	// Already logged in? Skip the form.
	if validSession(r) {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	renderLogin(w, next, "")
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// sanitizeNext keeps post-login redirects on this site: only local, absolute
// paths are allowed, defaulting to "/". Blocks open-redirect via ?next=.
func sanitizeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	return next
}

// handleSave persists an edit from the web editor. The hidden "mode" field
// says whether this is a new entry (slug optional, derived from the title if
// left blank) or an edit of an existing one (slug fixed, loaded first so its
// creation date is preserved).
func handleSave(w http.ResponseWriter, r *http.Request, kbDir string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	isNew := r.FormValue("mode") != "edit"
	slug := strings.TrimSpace(r.FormValue("slug"))
	title := strings.TrimSpace(r.FormValue("title"))
	body := r.FormValue("body")
	tags := parseTags(r.FormValue("tags"))
	section := strings.TrimSpace(r.FormValue("section"))

	// renderErr re-renders the form being submitted, preserving section
	// context (if any) so a resubmission after fixing the error still
	// targets the same section instead of silently becoming a full-body
	// save.
	renderErr := func(errMsg string) {
		sectionHeading := ""
		if section != "" {
			if existing, err := kb.Load(kbDir, slug); err == nil {
				secs := splitSections(existing.Body)
				if idx, err := strconv.Atoi(section); err == nil && idx >= 0 && idx < len(secs) {
					sectionHeading = strings.TrimSpace(strings.TrimLeft(secs[idx].Heading, "#"))
				}
			}
		}
		renderEditView(w, editView{
			IsNew: isNew, Slug: slug, Title: title, Tags: tags, Body: body,
			Err: errMsg, Section: section, SectionHeading: sectionHeading,
			Attachments: kb.ListAttachments(kbDir, slug),
		})
	}

	if slug != "" && !kb.ValidSlug(slug) {
		renderErr("slug may only contain letters, numbers, hyphens, and underscores")
		return
	}
	if title == "" {
		renderErr("title is required")
		return
	}

	var e *kb.Entry
	if isNew {
		newSlug := slug
		if newSlug == "" {
			newSlug = kb.Slugify(title)
		}
		if newSlug == "" {
			renderErr("could not derive a slug from the title — type one in the slug field")
			return
		}
		if kb.Exists(kbDir, newSlug) {
			renderErr(fmt.Sprintf("entry %q already exists", newSlug))
			return
		}
		e = &kb.Entry{Slug: newSlug, Title: title, Tags: tags, Date: time.Now(), Body: body}
	} else {
		// Existing entry — preserve its original date.
		existing, err := kb.Load(kbDir, slug)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if section != "" {
			secs := splitSections(existing.Body)
			idx, convErr := strconv.Atoi(section)
			if convErr != nil || idx < 0 || idx >= len(secs) {
				http.Error(w, "this section changed since you started editing — reload the page and try again", http.StatusConflict)
				return
			}
			existing.Body = replaceSection(existing.Body, secs, idx, body)
		} else {
			existing.Body = body
		}
		existing.Title = title
		existing.Tags = tags
		e = existing
	}

	if err := kb.Save(kbDir, e); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/entry/"+e.Slug, http.StatusSeeOther)
}

// handleUpload stores one image under a flat, page-prefixed filename. Requiring
// the entry to exist prevents uploads from becoming detached from a page before
// the page's slug has been established.
func handleUpload(w http.ResponseWriter, r *http.Request, kbDir string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	slug := strings.TrimPrefix(r.URL.Path, "/upload/")
	if _, err := kb.Load(kbDir, slug); err != nil {
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, kb.MaxAttachmentSize+(1<<20))
	if err := r.ParseMultipartForm(kb.MaxAttachmentSize); err != nil {
		http.Error(w, "image is too large or the upload is invalid", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "image file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	a, err := kb.StoreAttachment(kbDir, slug, header.Filename, file)
	if err != nil {
		switch {
		case errors.Is(err, kb.ErrInvalidAttachmentSlug):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, kb.ErrUnsupportedImage):
			http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		case errors.Is(err, kb.ErrAttachmentTooLarge):
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		default:
			http.Error(w, "could not store image", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

// parseTags splits a comma-separated tag string into trimmed, non-empty tags.
func parseTags(raw string) []string {
	var tags []string
	for _, t := range strings.Split(raw, ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

func printListenAddrs() {
	if cfg.ServeUser == "" || cfg.ServePass == "" {
		fmt.Println("warning: serve_user/serve_pass not set in ~/.gkb — running WITHOUT authentication")
	}

	// When bound to a specific loopback/interface address, report just that.
	if serveBind != "0.0.0.0" && serveBind != "" {
		fmt.Printf("serving at http://%s:%d\n", serveBind, servePort)
		return
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Printf("serving on port %d\n", servePort)
		return
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil {
				continue
			}
			fmt.Printf("serving at http://%s:%d\n", ip, servePort)
		}
	}
}

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 8086, "port to listen on")
	serveCmd.Flags().StringVarP(&serveBind, "bind", "b", "0.0.0.0", "address to bind (use 127.0.0.1 behind a TLS reverse proxy)")
	rootCmd.AddCommand(serveCmd)
}

var listTmpl = template.Must(template.New("list").Parse(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>gkb</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; font-size: 15px; color: #222; background: #f9f9f7; }
.wrap { max-width: 720px; margin: 0 auto; padding: 2rem 1.5rem; }
h1 { font-size: 1.25rem; font-weight: 600; margin-bottom: 1.5rem; color: #111; }
form { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 1.5rem; }
input[type=text] { flex: 1; padding: 6px 10px; border: 1px solid #ddd; border-radius: 6px; font-size: 14px; background: #fff; }
button { padding: 6px 14px; border: 1px solid #ddd; border-radius: 6px; background: #fff; font-size: 14px; cursor: pointer; }
button:hover { background: #f0f0ee; }
.btn { padding: 6px 14px; border: 1px solid #ddd; border-radius: 6px; background: #fff; font-size: 14px; text-decoration: none; color: #222; display: inline-flex; align-items: center; }
.btn:hover { background: #f0f0ee; color: #222; }
ul { list-style: none; }
li { border-bottom: 1px solid #eee; padding: 10px 0; display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 12px; }
li:last-child { border-bottom: none; }
li > a { flex-shrink: 0; max-width: 100%; }
a { color: #1a1a1a; text-decoration: none; font-weight: 500; }
a:hover { color: #0066cc; }
.meta { font-size: 13px; color: #888; }
.stats { font-size: 13px; color: #888; margin-bottom: 1.25rem; }
.tag { display: inline-block; font-size: 12px; padding: 1px 7px; border-radius: 4px; background: #efefed; color: #555; margin-left: 4px; }
.tag a { color: #555; font-weight: 400; }
.empty { color: #888; font-size: 14px; }
.signout { font-size: 12px; font-weight: 400; color: #888; text-decoration: none; margin-left: 6px; }
.signout:hover { color: #0066cc; }
</style>
</head>
<body>
<div class="wrap">
<h1><a href="/">gkb</a>{{if .Authed}} <a class="signout" href="/logout">sign out</a>{{end}}</h1>
<p class="stats">{{.TotalEntries}} page{{if ne .TotalEntries 1}}s{{end}} · {{.TotalAttachments}} attachment{{if ne .TotalAttachments 1}}s{{end}}</p>
<form method="get" action="/">
  <input type="text" name="q" placeholder="search..." value="{{.Query}}">
  <button type="submit">search</button>
  <a class="btn" href="/new">+ new</a>
</form>
{{if .Entries}}
<ul>
{{range .Entries}}
<li>
  <a href="/entry/{{.Slug}}">{{.Title}}</a>
  <span class="meta">{{.Date.Format "2006-01-02"}}</span>
  {{range .Tags}}<span class="tag"><a href="/?tag={{.}}">{{.}}</a></span>{{end}}
</li>
{{end}}
</ul>
{{else}}
<p class="empty">no entries</p>
{{end}}
</div>
</body>
</html>`))

var entryTmpl = template.Must(template.New("entry").Parse(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — gkb</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; font-size: 15px; color: #222; background: #f9f9f7; }
.wrap { max-width: 720px; margin: 0 auto; padding: 2rem 1.5rem; }
.nav { font-size: 13px; margin-bottom: 1.5rem; }
.nav a { color: #0066cc; text-decoration: none; }
.nav a:hover { text-decoration: underline; }
h1 { font-size: 1.4rem; font-weight: 600; margin-bottom: 0.5rem; }
.meta { font-size: 13px; color: #888; margin-bottom: 1.5rem; }
.tag { display: inline-block; font-size: 12px; padding: 1px 7px; border-radius: 4px; background: #efefed; color: #555; margin-left: 4px; }
.tag a { color: #555; }
.body { line-height: 1.7; }
.body h1,.body h2,.body h3 { margin: 1.5rem 0 0.5rem; font-weight: 600; }
.body h1 { font-size: 1.2rem; }
.body h2 { font-size: 1.05rem; }
.body h3 { font-size: 0.95rem; }
.body p { margin-bottom: 1rem; }
.body ul,.body ol { margin: 0 0 1rem 1.5rem; }
.body li { margin-bottom: 0.25rem; }
.body code { font-family: monospace; font-size: 13px; background: #efefed; padding: 1px 5px; border-radius: 3px; }
.body pre { background: #efefed; padding: 1rem; border-radius: 6px; overflow-x: auto; margin-bottom: 1rem; }
.body pre code { background: none; padding: 0; }
.body a { color: #0066cc; }
.body img { max-width: 100%; height: auto; }
.body .math.display { display: block; overflow-x: auto; margin: 1rem 0; }
.body table { width: 100%; border-collapse: collapse; margin-bottom: 1.25rem; }
.body th, .body td { text-align: left; padding: 10px 20px 10px 0; vertical-align: top; }
.body th { font-weight: 600; color: #111; border-bottom: 2px solid #ddd; }
.body td { border-bottom: 1px solid #eee; }
.body tr:last-child td { border-bottom: none; }
.backlinks { font-size: 13px; color: #888; margin-top: 2rem; padding-top: 1rem; border-top: 1px solid #eee; }
.backlinks a { color: #0066cc; }
.section-edit { font-size: 0.7rem; font-weight: 400; color: #999; text-decoration: none; margin-left: 8px; vertical-align: middle; }
.section-edit:hover { color: #0066cc; }
</style>
<script>
window.MathJax = {
  tex: { inlineMath: [['\\(', '\\)']], displayMath: [['\\[', '\\]']] },
  options: { skipHtmlTags: ['script', 'noscript', 'style', 'textarea', 'pre', 'code'] }
};
</script>
<script async src="https://cdn.jsdelivr.net/npm/mathjax@3/es5/tex-mml-chtml.js"></script>
</head>
<body>
<div class="wrap">
<div class="nav"><a href="/">← all entries</a> · <a href="/edit/{{.Slug}}">edit</a></div>
<h1>{{.Title}}</h1>
<div class="meta">
  {{.Date.Format "2006-01-02"}}
  {{range .Tags}}<span class="tag"><a href="/?tag={{.}}">{{.}}</a></span>{{end}}
</div>
<div class="body">{{.HTML}}</div>
{{if .Backlinks}}
<p class="backlinks">Linked from: {{range $i, $b := .Backlinks}}{{if $i}}, {{end}}<a href="/entry/{{$b.Slug}}">{{$b.Title}}</a>{{end}}</p>
{{end}}
</div>
</body>
</html>`))

func renderList(w http.ResponseWriter, entries []*kb.Entry, query, tag string, totalEntries, totalAttachments int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	listTmpl.Execute(w, map[string]any{
		"Entries":          entries,
		"Query":            query,
		"Tag":              tag,
		"Authed":           cfg.ServeUser != "" && cfg.ServePass != "",
		"TotalEntries":     totalEntries,
		"TotalAttachments": totalAttachments,
	})
}

// renderNewFromWikiLink offers to create the page a broken [[wikilink]]
// pointed at, instead of a bare 404. raw is either an ASCII slug (the older
// [[slug]] convention — prefilled as Slug, so existing cross-references
// keep resolving once the page is saved) or free-form title text (the
// title-based convention — prefilled as Title, since slugs stay ASCII by
// design and the user needs to supply one). See
// decisions/004-wikilinks-by-title.md.
func renderNewFromWikiLink(w http.ResponseWriter, raw, kbDir string) {
	e := &kb.Entry{}
	if kb.ValidSlug(raw) {
		e.Slug = raw
	} else {
		e.Title = raw
	}
	renderEdit(w, e, true, "", kbDir)
}

func renderEntry(w http.ResponseWriter, e *kb.Entry, kbDir string) {
	entries, _ := kb.List(kbDir)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	entryTmpl.Execute(w, map[string]any{
		"Title":     e.Title,
		"Date":      e.Date,
		"Tags":      e.Tags,
		"Slug":      e.Slug,
		"HTML":      template.HTML(addSectionEditLinks(renderMarkdown(e.Body, entries), e.Slug)),
		"Backlinks": backlinks(entries, e.Slug),
	})
}

var editTmpl = template.Must(template.New("edit").Parse(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{if .IsNew}}new entry{{else}}editing {{.Title}}{{end}} — gkb</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; font-size: 15px; color: #222; background: #f9f9f7; }
.wrap { max-width: 720px; margin: 0 auto; padding: 2rem 1.5rem; }
.nav { font-size: 13px; margin-bottom: 1.5rem; }
.nav a { color: #0066cc; text-decoration: none; }
.nav a:hover { text-decoration: underline; }
h1 { font-size: 1.4rem; font-weight: 600; margin-bottom: 1.5rem; }
label { display: block; font-size: 13px; color: #555; margin-bottom: 4px; }
.field { margin-bottom: 1.25rem; }
input[type=text], textarea { width: 100%; padding: 8px 10px; border: 1px solid #ddd; border-radius: 6px; font-size: 14px; background: #fff; font-family: inherit; }
textarea { min-height: 22rem; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; line-height: 1.5; resize: vertical; }
.actions { display: flex; gap: 8px; align-items: center; }
button { padding: 7px 16px; border: 1px solid #0066cc; border-radius: 6px; background: #0066cc; color: #fff; font-size: 14px; cursor: pointer; }
button:hover { background: #0055aa; }
.cancel { font-size: 14px; color: #888; text-decoration: none; }
.cancel:hover { text-decoration: underline; }
.err { color: #b00020; font-size: 14px; margin-bottom: 1rem; }
.hint { font-size: 12px; color: #999; margin-top: 4px; }
.dropzone { border: 2px dashed #ccc; border-radius: 6px; padding: 1.25rem; text-align: center; color: #777; cursor: pointer; background: #fff; }
.dropzone.dragging { border-color: #0066cc; background: #f2f8ff; color: #0066cc; }
.dropzone input { display: none; }
.upload-status { min-height: 1.2em; font-size: 12px; color: #777; margin-top: 5px; }
.attachments { list-style: none; margin-top: 8px; }
.attachments li { display: flex; gap: 8px; align-items: center; margin-bottom: 5px; }
.attachments .thumb { width: 36px; height: 36px; object-fit: cover; border-radius: 4px; border: 1px solid #ddd; flex-shrink: 0; background: #efefed; }
.attachments code { flex: 1; overflow-wrap: anywhere; padding: 5px 7px; border-radius: 4px; background: #efefed; font-size: 12px; }
.copy { padding: 4px 9px; border-color: #ccc; background: #fff; color: #333; font-size: 12px; }
.copy:hover { background: #f0f0ee; }
.editor-toolbar { margin-bottom: 6px; }
.toolbar-btn { padding: 4px 9px; border-color: #ccc; background: #fff; color: #333; font-size: 12px; }
.toolbar-btn:hover { background: #f0f0ee; }
</style>
</head>
<body>
<div class="wrap">
<div class="nav"><a href="/">← all entries</a>{{if not .IsNew}} · <a href="/entry/{{.Slug}}">view</a>{{end}}{{if .Section}} · <a href="/edit/{{.Slug}}">edit full page</a>{{end}}</div>
<h1>{{if .IsNew}}new entry{{else if .Section}}editing section: {{.SectionHeading}}{{else}}edit{{end}}</h1>
{{if .Err}}<p class="err">{{.Err}}</p>{{end}}
<form method="post" action="/save">
  <input type="hidden" name="mode" value="{{if .IsNew}}new{{else}}edit{{end}}">
  {{if not .IsNew}}<input type="hidden" name="slug" value="{{.Slug}}">{{end}}
  {{if .Section}}<input type="hidden" name="section" value="{{.Section}}">{{end}}
  <div class="field">
    <label>title</label>
    <input type="text" name="title" value="{{.Title}}" autofocus>
    {{if not .IsNew}}<p class="hint">slug: {{.Slug}} (fixed — use the CLI to rename)</p>{{end}}
  </div>
  {{if .IsNew}}
  <div class="field">
    <label>slug</label>
    <input type="text" name="slug" value="{{.Slug}}" placeholder="derived from the title if left blank">
    <p class="hint">only needed if the title isn't ASCII, or to pick something else</p>
  </div>
  {{end}}
  <div class="field">
    <label>tags (comma-separated)</label>
    <input type="text" name="tags" value="{{.TagStr}}">
  </div>
  <div class="field">
    <label>body (markdown)</label>
    <div class="editor-toolbar">
      <button type="button" class="toolbar-btn" id="insert-table">+ table</button>
    </div>
    <textarea name="body" spellcheck="false">{{.Body}}</textarea>
    <p class="hint">in a table: Tab/Shift+Tab moves between cells, Enter on the last cell adds a row</p>
    {{if .Section}}<p class="hint">only this section will be saved — the rest of the page is untouched</p>{{end}}
  </div>
  {{if not .IsNew}}
  <div class="field">
    <label>images</label>
    <div class="dropzone" id="dropzone">
      drop, paste, or click to choose an image
      <input id="image-input" type="file" accept="image/jpeg,image/png,image/gif,image/webp">
    </div>
    <p class="upload-status" id="upload-status"></p>
    <ul class="attachments" id="attachments">
      {{range .Attachments}}<li><img class="thumb" src="/attachments/{{.Name}}" alt=""><code>{{.Markup}}</code><button class="copy" type="button">copy</button></li>{{end}}
    </ul>
  </div>
  {{end}}
  <div class="actions">
    <button type="submit">save</button>
    <a class="cancel" href="{{if .IsNew}}/{{else}}/entry/{{.Slug}}{{end}}">cancel</a>
  </div>
</form>
</div>
<script>
(function() {
  const bodyField = document.querySelector('textarea[name=body]');

  function rowCells(line) {
    let t = line.trim();
    if (t.charAt(0) === '|') t = t.slice(1);
    if (t.charAt(t.length - 1) === '|') t = t.slice(0, -1);
    return t.split('|');
  }

  function isSeparatorRow(line) {
    const cells = rowCells(line);
    return cells.length > 0 && cells.every(c => /^\s*:?-+:?\s*$/.test(c));
  }

  function isTableRow(line) {
    const t = line.trim();
    return t.length >= 2 && t.charAt(0) === '|' && t.charAt(t.length - 1) === '|' && !isSeparatorRow(line);
  }

  function emptyRow(cols) {
    return '|' + new Array(cols).fill('  ').join('|') + '|';
  }

  function pipePositions(line) {
    const positions = [];
    for (let i = 0; i < line.length; i++) if (line.charAt(i) === '|') positions.push(i);
    return positions;
  }

  function currentLine(pos) {
    const value = bodyField.value;
    const start = value.lastIndexOf('\n', pos - 1) + 1;
    const stop = value.indexOf('\n', pos);
    return { text: value.slice(start, stop === -1 ? value.length : stop), start };
  }

  function insertTable() {
    const colsInput = prompt('Number of columns?', '3');
    if (colsInput === null) return;
    const cols = Math.max(1, parseInt(colsInput, 10) || 3);
    const rowsInput = prompt('Number of data rows (not counting the header)?', '2');
    if (rowsInput === null) return;
    const rows = Math.max(1, parseInt(rowsInput, 10) || 2);

    const header = [];
    const sep = [];
    for (let i = 0; i < cols; i++) {
      header.push(' Header ' + (i + 1) + ' ');
      sep.push(' --- ');
    }
    const lines = ['|' + header.join('|') + '|', '|' + sep.join('|') + '|'];
    for (let r = 0; r < rows; r++) lines.push(emptyRow(cols));
    const table = lines.join('\n') + '\n';

    const start = bodyField.selectionStart;
    const before = bodyField.value.slice(0, start);
    const needsLeadingNewline = before.length > 0 && before.charAt(before.length - 1) !== '\n';
    bodyField.setRangeText((needsLeadingNewline ? '\n' : '') + table, start, bodyField.selectionEnd, 'end');
    bodyField.focus();
  }

  const insertTableBtn = document.getElementById('insert-table');
  if (insertTableBtn) insertTableBtn.addEventListener('click', insertTable);

  bodyField.addEventListener('keydown', e => {
    if (e.key !== 'Enter' && e.key !== 'Tab') return;
    const pos = bodyField.selectionStart;
    const line = currentLine(pos);
    if (!isTableRow(line.text)) return;

    if (e.key === 'Enter') {
      if (pos !== line.start + line.text.length) return; // only at end of the row
      e.preventDefault();
      const cols = rowCells(line.text).length;
      const newRow = '\n' + emptyRow(cols);
      bodyField.setRangeText(newRow, pos, pos, 'end');
      bodyField.setSelectionRange(pos + 2, pos + 4); // select the new first cell's placeholder
      return;
    }

    // Tab: move between cells of this row, wrapping at either end.
    e.preventDefault();
    const positions = pipePositions(line.text);
    let cell = 0;
    for (let i = 0; i < positions.length - 1; i++) {
      // Strict ">" so a caret sitting exactly on a pipe (e.g. right after
      // typing replacement text that fills a cell) still counts as that
      // pipe's left-hand cell, not the one after it.
      if (pos - line.start > positions[i]) cell = i;
    }
    cell += e.shiftKey ? -1 : 1;
    const count = positions.length - 1;
    cell = (cell + count) % count;
    const cellStart = line.start + positions[cell] + 1;
    const cellEnd = line.start + positions[cell + 1];
    bodyField.setSelectionRange(cellStart, cellEnd);
  });
})();
</script>
{{if not .IsNew}}
<script>
const zone = document.getElementById('dropzone');
const input = document.getElementById('image-input');
const status = document.getElementById('upload-status');
const list = document.getElementById('attachments');

function addAttachment(attachment) {
  const li = document.createElement('li');
  const thumb = document.createElement('img');
  const code = document.createElement('code');
  const button = document.createElement('button');
  thumb.className = 'thumb';
  thumb.src = '/attachments/' + attachment.name;
  thumb.alt = '';
  code.textContent = attachment.markup;
  button.type = 'button';
  button.className = 'copy';
  button.textContent = 'copy';
  li.append(thumb, code, button);
  list.append(li);
}

async function uploadImage(file) {
  status.textContent = 'uploading…';
  const data = new FormData();
  data.append('image', file);
  try {
    const response = await fetch('/upload/{{.Slug}}', {method: 'POST', body: data});
    if (!response.ok) throw new Error((await response.text()).trim());
    const attachment = await response.json();
    addAttachment(attachment);
    status.textContent = 'uploaded';
    return attachment;
  } catch (error) {
    status.textContent = error.message || 'upload failed';
    return null;
  }
}

async function upload(file) {
  if (!file) return;
  await uploadImage(file);
  input.value = '';
}

zone.addEventListener('click', () => input.click());
input.addEventListener('change', () => upload(input.files[0]));
for (const event of ['dragenter', 'dragover']) {
  zone.addEventListener(event, e => { e.preventDefault(); zone.classList.add('dragging'); });
}
for (const event of ['dragleave', 'drop']) {
  zone.addEventListener(event, e => { e.preventDefault(); zone.classList.remove('dragging'); });
}
zone.addEventListener('drop', e => upload(e.dataTransfer.files[0]));

const bodyField = document.querySelector('textarea[name=body]');
bodyField.addEventListener('paste', async e => {
  const items = e.clipboardData && e.clipboardData.items;
  if (!items) return;
  for (const item of items) {
    if (item.kind !== 'file' || !item.type.startsWith('image/')) continue;
    e.preventDefault();
    const file = item.getAsFile();
    const attachment = await uploadImage(file);
    if (attachment) bodyField.setRangeText(attachment.markup, bodyField.selectionStart, bodyField.selectionEnd, 'end');
    break;
  }
});
list.addEventListener('click', async e => {
  if (!e.target.classList.contains('copy')) return;
  const markup = e.target.previousElementSibling.textContent;
  try {
    if (navigator.clipboard) {
      await navigator.clipboard.writeText(markup);
    } else {
      const temporary = document.createElement('textarea');
      temporary.value = markup;
      temporary.style.position = 'fixed';
      temporary.style.opacity = '0';
      document.body.append(temporary);
      temporary.select();
      document.execCommand('copy');
      temporary.remove();
    }
    e.target.textContent = 'copied';
    setTimeout(() => { e.target.textContent = 'copy'; }, 1200);
  } catch (_) {
    status.textContent = 'copy failed; select the Markdown manually';
  }
});
</script>
{{end}}
</body>
</html>`))

// editView holds everything the edit template needs, so both a full-page
// edit and a single-section edit (see renderEditSection) can share one
// renderer.
type editView struct {
	IsNew          bool
	Slug           string
	Title          string
	Tags           []string
	Body           string
	Err            string
	Section        string // "" for a full-page edit, else the section index
	SectionHeading string
	Attachments    []kb.Attachment
}

func renderEditView(w http.ResponseWriter, v editView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	editTmpl.Execute(w, map[string]any{
		"IsNew":          v.IsNew,
		"Slug":           v.Slug,
		"Title":          v.Title,
		"TagStr":         strings.Join(v.Tags, ", "),
		"Body":           v.Body,
		"Err":            v.Err,
		"Section":        v.Section,
		"SectionHeading": v.SectionHeading,
		"Attachments":    v.Attachments,
	})
}

func renderEdit(w http.ResponseWriter, e *kb.Entry, isNew bool, errMsg, kbDir string) {
	renderEditView(w, editView{
		IsNew:       isNew,
		Slug:        e.Slug,
		Title:       e.Title,
		Tags:        e.Tags,
		Body:        e.Body,
		Err:         errMsg,
		Attachments: kb.ListAttachments(kbDir, e.Slug),
	})
}

// renderEditSection renders the editor scoped to one heading section (see
// splitSections), reached via the "edit" links addSectionEditLinks adds to
// each heading on the entry page.
func renderEditSection(w http.ResponseWriter, e *kb.Entry, secs []mdSection, idx int, errMsg, kbDir string) {
	renderEditView(w, editView{
		Slug:           e.Slug,
		Title:          e.Title,
		Tags:           e.Tags,
		Body:           sectionText(e.Body, secs, idx),
		Err:            errMsg,
		Section:        strconv.Itoa(idx),
		SectionHeading: strings.TrimSpace(strings.TrimLeft(secs[idx].Heading, "#")),
		Attachments:    kb.ListAttachments(kbDir, e.Slug),
	})
}

var loginTmpl = template.Must(template.New("login").Parse(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>sign in — gkb</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; font-size: 15px; color: #222; background: #f9f9f7; }
.wrap { max-width: 340px; margin: 12vh auto 0; padding: 2rem 1.5rem; }
h1 { font-size: 1.25rem; font-weight: 600; margin-bottom: 1.5rem; color: #111; }
.field { margin-bottom: 1rem; }
label { display: block; font-size: 13px; color: #555; margin-bottom: 4px; }
input { width: 100%; padding: 8px 10px; border: 1px solid #ddd; border-radius: 6px; font-size: 14px; background: #fff; }
button { width: 100%; padding: 9px 16px; border: 1px solid #0066cc; border-radius: 6px; background: #0066cc; color: #fff; font-size: 14px; cursor: pointer; margin-top: 0.5rem; }
button:hover { background: #0055aa; }
.err { color: #b00020; font-size: 14px; margin-bottom: 1rem; }
</style>
</head>
<body>
<div class="wrap">
<h1>gkb</h1>
{{if .Err}}<p class="err">{{.Err}}</p>{{end}}
<form method="post" action="/login">
  <input type="hidden" name="next" value="{{.Next}}">
  <div class="field">
    <label>username</label>
    <input type="text" name="username" autocomplete="username" autofocus>
  </div>
  <div class="field">
    <label>password</label>
    <input type="password" name="password" autocomplete="current-password">
  </div>
  <button type="submit">sign in</button>
</form>
</div>
</body>
</html>`))

func renderLogin(w http.ResponseWriter, next, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if errMsg != "" {
		w.WriteHeader(http.StatusUnauthorized)
	}
	loginTmpl.Execute(w, map[string]any{"Next": next, "Err": errMsg})
}

var wikiLink = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)

// wikiLinkIndex resolves a [[target]] to the slug it refers to: target is
// tried as a slug first (existing behavior, unchanged); if that doesn't
// match any entry, it's tried against every entry's Title instead, so a
// non-ASCII-titled page (slugs are kept ASCII by design) can be linked by
// its title directly — e.g. [[ローマ]] — without knowing or repeating its
// slug. See decisions/004-wikilinks-by-title.md.
type wikiLinkIndex struct {
	slugs  map[string]bool
	titles map[string]string
}

func buildWikiLinkIndex(entries []*kb.Entry) *wikiLinkIndex {
	idx := &wikiLinkIndex{
		slugs:  make(map[string]bool, len(entries)),
		titles: make(map[string]string, len(entries)),
	}
	for _, e := range entries {
		idx.slugs[e.Slug] = true
		if e.Title != "" {
			if _, exists := idx.titles[e.Title]; !exists {
				idx.titles[e.Title] = e.Slug
			}
		}
	}
	return idx
}

// resolve returns the slug target refers to, falling back to target itself
// (today's graceful-degradation behavior for a page that doesn't exist yet).
func (idx *wikiLinkIndex) resolve(target string) string {
	if idx.slugs[target] {
		return target
	}
	if slug, ok := idx.titles[target]; ok {
		return slug
	}
	return target
}

// expandWikiLinks turns [[target]] / [[target|text]] into Markdown links.
func expandWikiLinks(body string, entries []*kb.Entry) string {
	idx := buildWikiLinkIndex(entries)
	return wikiLink.ReplaceAllStringFunc(body, func(match string) string {
		m := wikiLink.FindStringSubmatch(match)
		target, text := m[1], m[2]
		if text == "" {
			text = target
		}
		return fmt.Sprintf("[%s](/entry/%s)", text, idx.resolve(target))
	})
}

// backlinks returns entries whose body contains a [[wikilink]] resolving to
// slug (same resolution rules as expandWikiLinks), for display as "linked
// from" at the bottom of an entry page.
func backlinks(entries []*kb.Entry, slug string) []*kb.Entry {
	idx := buildWikiLinkIndex(entries)
	var links []*kb.Entry
	for _, e := range entries {
		if e.Slug == slug {
			continue
		}
		for _, m := range wikiLink.FindAllStringSubmatch(e.Body, -1) {
			if idx.resolve(m[1]) == slug {
				links = append(links, e)
				break
			}
		}
	}
	return links
}

// mdSection is a heading and everything under it, including nested
// subsections — i.e. it runs from its heading line down to (but not
// including) the next heading of the same or shallower level, or the end of
// the document. Start/End are line indices into the body it was split from.
type mdSection struct {
	Level   int
	Heading string
	Start   int
	End     int
}

var (
	headingLine = regexp.MustCompile(`^(#{1,6})\s+\S`)
	fenceLine   = regexp.MustCompile("^(```|~~~)")
)

// splitSections finds heading-delimited sections in body, skipping any '#'
// that appears inside a fenced code block (e.g. a markdown example inside
// ```). The result mirrors the <h1>-<h6> tags renderMarkdown will produce
// for the same body, in the same order, which lets section-edit links use a
// plain positional index (see addSectionEditLinks).
func splitSections(body string) []mdSection {
	lines := strings.Split(body, "\n")
	var secs []mdSection
	inFence := false
	for i, line := range lines {
		if fenceLine.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := headingLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		secs = append(secs, mdSection{Level: len(m[1]), Heading: line, Start: i, End: len(lines)})
	}
	for i := range secs {
		for j := i + 1; j < len(secs); j++ {
			if secs[j].Level <= secs[i].Level {
				secs[i].End = secs[j].Start
				break
			}
		}
	}
	return secs
}

// trimBlankLines drops leading and trailing blank lines, so callers that
// splice sections back together can add back exactly the separator they
// need instead of depending on whichever whitespace happened to survive
// editing.
func trimBlankLines(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

// sectionText returns the raw markdown of secs[idx], heading line included,
// with the blank separator line before the next section trimmed off (it's
// not meaningful content — replaceSection restores exactly one).
func sectionText(body string, secs []mdSection, idx int) string {
	lines := strings.Split(body, "\n")
	return strings.Join(trimBlankLines(lines[secs[idx].Start:secs[idx].End]), "\n")
}

// replaceSection splices replacement in place of secs[idx] within body,
// leaving everything else untouched. A blank line is always inserted
// between the replacement and whatever section follows, regardless of
// trailing whitespace in replacement — otherwise a replacement that doesn't
// end in a blank line gets glued directly onto the next heading, breaking
// it (e.g. "new content\n# b" instead of "new content\n\n# b").
func replaceSection(body string, secs []mdSection, idx int, replacement string) string {
	lines := strings.Split(body, "\n")
	rest := lines[secs[idx].End:]

	var out []string
	out = append(out, lines[:secs[idx].Start]...)
	out = append(out, trimBlankLines(strings.Split(replacement, "\n"))...)
	if len(rest) > 0 {
		out = append(out, "")
	}
	out = append(out, rest...)
	return strings.Join(out, "\n")
}

var closeHeadingTag = regexp.MustCompile(`</h[1-6]>`)

// addSectionEditLinks inserts a small "edit" link at the end of each
// rendered heading, pointing at /edit/<slug>?section=<n>. n is a running
// count over headings in document order, matching splitSections' indexing.
func addSectionEditLinks(renderedHTML, slug string) string {
	n := -1
	return closeHeadingTag.ReplaceAllStringFunc(renderedHTML, func(closeTag string) string {
		n++
		return fmt.Sprintf(` <a class="section-edit" href="/edit/%s?section=%d">edit</a>%s`, slug, n, closeTag)
	})
}

var (
	blockMath  = regexp.MustCompile(`(?s)\\\[(.+?)\\\]`)
	inlineMath = regexp.MustCompile(`\\\((.+?)\\\)`)
)

var mathEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// protectMath pulls LaTeX math spans (\(...\) inline, \[...\] block) out of the
// markdown source before rendering so the processor can't mangle characters like
// _, *, and \ inside the formulas. Each span is replaced with an alphanumeric
// placeholder (untouched by markdown) and returned as MathJax-ready HTML to
// splice back after rendering.
func protectMath(body string) (string, []string) {
	var spans []string
	protect := func(re *regexp.Regexp, open, close string) {
		body = re.ReplaceAllStringFunc(body, func(match string) string {
			inner := mathEscaper.Replace(re.FindStringSubmatch(match)[1])
			token := fmt.Sprintf("zzzgkbmath%dzzz", len(spans))
			spans = append(spans, open+inner+close)
			return token
		})
	}
	protect(blockMath, `<span class="math display">\[`, `\]</span>`)
	protect(inlineMath, `<span class="math inline">\(`, `\)</span>`)
	return body, spans
}

func renderMarkdown(body string, entries []*kb.Entry) string {
	body = expandWikiLinks(body, entries)
	body = strings.ReplaceAll(body, "](attachments/", "](/attachments/")
	body, spans := protectMath(body)
	p := parser.NewWithExtensions(parser.CommonExtensions | parser.AutoHeadingIDs)
	r := html.NewRenderer(html.RendererOptions{Flags: html.CommonFlags})
	out := string(markdown.ToHTML([]byte(body), p, r))
	for i, s := range spans {
		out = strings.Replace(out, fmt.Sprintf("zzzgkbmath%dzzz", i), s, 1)
	}
	return out
}
