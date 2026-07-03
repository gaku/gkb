package cmd

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
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

		mux := http.NewServeMux()

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
			renderList(w, entries, q, tag)
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
				http.NotFound(w, r)
				return
			}
			renderEntry(w, e)
		})

		mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
			renderEdit(w, &kb.Entry{}, true, "")
		})

		mux.HandleFunc("/edit/", func(w http.ResponseWriter, r *http.Request) {
			slug := strings.TrimPrefix(r.URL.Path, "/edit/")
			e, err := kb.Load(kbDir, slug)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			renderEdit(w, e, false, "")
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

// handleSave persists an edit from the web editor. An empty slug field means a
// new entry (slug derived from the title); otherwise the existing entry is
// loaded so its creation date is preserved and its title/tags/body updated.
func handleSave(w http.ResponseWriter, r *http.Request, kbDir string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	slug := strings.TrimSpace(r.FormValue("slug"))
	title := strings.TrimSpace(r.FormValue("title"))
	body := r.FormValue("body")
	tags := parseTags(r.FormValue("tags"))

	if title == "" {
		renderEdit(w, &kb.Entry{Slug: slug, Title: title, Tags: tags, Body: body}, slug == "", "title is required")
		return
	}

	var e *kb.Entry
	if slug == "" {
		// New entry.
		newSlug := kb.Slugify(title)
		if newSlug == "" {
			renderEdit(w, &kb.Entry{Title: title, Tags: tags, Body: body}, true, "could not derive a slug from the title")
			return
		}
		if kb.Exists(kbDir, newSlug) {
			renderEdit(w, &kb.Entry{Title: title, Tags: tags, Body: body}, true, fmt.Sprintf("entry %q already exists", newSlug))
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
		existing.Title = title
		existing.Tags = tags
		existing.Body = body
		e = existing
	}

	if err := kb.Save(kbDir, e); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/entry/"+e.Slug, http.StatusSeeOther)
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
<title>gkb</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; font-size: 15px; color: #222; background: #f9f9f7; }
.wrap { max-width: 720px; margin: 0 auto; padding: 2rem 1.5rem; }
h1 { font-size: 1.25rem; font-weight: 600; margin-bottom: 1.5rem; color: #111; }
form { display: flex; gap: 8px; margin-bottom: 1.5rem; }
input[type=text] { flex: 1; padding: 6px 10px; border: 1px solid #ddd; border-radius: 6px; font-size: 14px; background: #fff; }
button { padding: 6px 14px; border: 1px solid #ddd; border-radius: 6px; background: #fff; font-size: 14px; cursor: pointer; }
button:hover { background: #f0f0ee; }
.btn { padding: 6px 14px; border: 1px solid #ddd; border-radius: 6px; background: #fff; font-size: 14px; text-decoration: none; color: #222; display: inline-flex; align-items: center; }
.btn:hover { background: #f0f0ee; color: #222; }
ul { list-style: none; }
li { border-bottom: 1px solid #eee; padding: 10px 0; display: flex; align-items: baseline; gap: 12px; }
li:last-child { border-bottom: none; }
a { color: #1a1a1a; text-decoration: none; font-weight: 500; }
a:hover { color: #0066cc; }
.meta { font-size: 13px; color: #888; }
.tag { display: inline-block; font-size: 12px; padding: 1px 7px; border-radius: 4px; background: #efefed; color: #555; margin-left: 4px; }
.tag a { color: #555; font-weight: 400; }
.empty { color: #888; font-size: 14px; }
.signout { font-size: 12px; font-weight: 400; color: #888; text-decoration: none; margin-left: 6px; }
.signout:hover { color: #0066cc; }
</style>
</head>
<body>
<div class="wrap">
<h1>gkb{{if .Authed}} <a class="signout" href="/logout">sign out</a>{{end}}</h1>
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
.body .math.display { display: block; overflow-x: auto; margin: 1rem 0; }
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
</div>
</body>
</html>`))

func renderList(w http.ResponseWriter, entries []*kb.Entry, query, tag string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	listTmpl.Execute(w, map[string]any{
		"Entries": entries,
		"Query":   query,
		"Tag":     tag,
		"Authed":  cfg.ServeUser != "" && cfg.ServePass != "",
	})
}

func renderEntry(w http.ResponseWriter, e *kb.Entry) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	entryTmpl.Execute(w, map[string]any{
		"Title": e.Title,
		"Date":  e.Date,
		"Tags":  e.Tags,
		"Slug":  e.Slug,
		"HTML":  template.HTML(renderMarkdown(e.Body)),
	})
}

var editTmpl = template.Must(template.New("edit").Parse(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
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
</style>
</head>
<body>
<div class="wrap">
<div class="nav"><a href="/">← all entries</a>{{if not .IsNew}} · <a href="/entry/{{.Slug}}">view</a>{{end}}</div>
<h1>{{if .IsNew}}new entry{{else}}edit{{end}}</h1>
{{if .Err}}<p class="err">{{.Err}}</p>{{end}}
<form method="post" action="/save">
  <input type="hidden" name="slug" value="{{.Slug}}">
  <div class="field">
    <label>title</label>
    <input type="text" name="title" value="{{.Title}}" autofocus>
    {{if not .IsNew}}<p class="hint">slug: {{.Slug}} (fixed — use the CLI to rename)</p>{{end}}
  </div>
  <div class="field">
    <label>tags (comma-separated)</label>
    <input type="text" name="tags" value="{{.TagStr}}">
  </div>
  <div class="field">
    <label>body (markdown)</label>
    <textarea name="body" spellcheck="false">{{.Body}}</textarea>
  </div>
  <div class="actions">
    <button type="submit">save</button>
    <a class="cancel" href="{{if .IsNew}}/{{else}}/entry/{{.Slug}}{{end}}">cancel</a>
  </div>
</form>
</div>
</body>
</html>`))

func renderEdit(w http.ResponseWriter, e *kb.Entry, isNew bool, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	editTmpl.Execute(w, map[string]any{
		"IsNew":  isNew,
		"Slug":   e.Slug,
		"Title":  e.Title,
		"TagStr": strings.Join(e.Tags, ", "),
		"Body":   e.Body,
		"Err":    errMsg,
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

var wikiLink = regexp.MustCompile(`\[\[([a-zA-Z0-9_-]+)(?:\|([^\]]+))?\]\]`)

func expandWikiLinks(body string) string {
	return wikiLink.ReplaceAllStringFunc(body, func(match string) string {
		m := wikiLink.FindStringSubmatch(match)
		slug, text := m[1], m[2]
		if text == "" {
			text = slug
		}
		return fmt.Sprintf("[%s](/entry/%s)", text, slug)
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

func renderMarkdown(body string) string {
	body = expandWikiLinks(body)
	body, spans := protectMath(body)
	p := parser.NewWithExtensions(parser.CommonExtensions | parser.AutoHeadingIDs)
	r := html.NewRenderer(html.RendererOptions{Flags: html.CommonFlags})
	out := string(markdown.ToHTML([]byte(body), p, r))
	for i, s := range spans {
		out = strings.Replace(out, fmt.Sprintf("zzzgkbmath%dzzz", i), s, 1)
	}
	return out
}
