package cmd

import (
	"fmt"
	"html/template"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/gaku/gkb/internal/kb"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/spf13/cobra"
)

var servePort int

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start a local web server to browse the knowledge base",
	RunE: func(cmd *cobra.Command, args []string) error {
		kbDir := requireKbDir()

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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

		http.HandleFunc("/entry/", func(w http.ResponseWriter, r *http.Request) {
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

		addr := fmt.Sprintf("0.0.0.0:%d", servePort)
		printListenAddrs(servePort)
		return http.ListenAndServe(addr, nil)
	},
}

func printListenAddrs(port int) {
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Printf("serving on port %d\n", port)
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
			fmt.Printf("serving at http://%s:%d\n", ip, port)
		}
	}
}

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 8086, "port to listen on")
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
ul { list-style: none; }
li { border-bottom: 1px solid #eee; padding: 10px 0; display: flex; align-items: baseline; gap: 12px; }
li:last-child { border-bottom: none; }
a { color: #1a1a1a; text-decoration: none; font-weight: 500; }
a:hover { color: #0066cc; }
.meta { font-size: 13px; color: #888; }
.tag { display: inline-block; font-size: 12px; padding: 1px 7px; border-radius: 4px; background: #efefed; color: #555; margin-left: 4px; }
.tag a { color: #555; font-weight: 400; }
.empty { color: #888; font-size: 14px; }
</style>
</head>
<body>
<div class="wrap">
<h1>gkb</h1>
<form method="get" action="/">
  <input type="text" name="q" placeholder="search..." value="{{.Query}}">
  <button type="submit">search</button>
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
<div class="nav"><a href="/">← all entries</a></div>
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
