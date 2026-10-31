package report

import (
	"embed"
	"encoding/json"
	"html"
	"io"
	"path/filepath"
	"strings"

	"pmm-dump-info/internal/inspect"
)

//go:embed assets/page.html assets/app.js assets/style.css
var assets embed.FS

// WriteHTML writes the self-contained page: data, script and styles inline,
// no external requests, so customer data stays on the machine.
func WriteHTML(w io.Writer, r *inspect.Result) error {
	page, err := assets.ReadFile("assets/page.html")
	if err != nil {
		return err
	}
	js, err := assets.ReadFile("assets/app.js")
	if err != nil {
		return err
	}
	css, err := assets.ReadFile("assets/style.css")
	if err != nil {
		return err
	}
	// json.Marshal escapes <, > and &, so the data cannot close the script tag.
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	out := strings.NewReplacer(
		"{{TITLE}}", html.EscapeString(filepath.Base(r.File)),
		"{{CSS}}", string(css),
		"{{JS}}", string(js),
		"{{DATA}}", string(data),
	).Replace(string(page))
	_, err = io.WriteString(w, out)
	return err
}
