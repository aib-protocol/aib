package render

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
)

type Engine struct {
	templates map[string]*template.Template // "en:home.html" -> compiled template
}

var languages = []string{"en", "zh"}

var pages = []string{
	"home.html",
	"constitution.html",
	"plan.html",
	"genesis.html",
	"roadmap.html",
	"standards.html",
	"changelog.html",
	"ai-context.html",
}

var funcMap = template.FuncMap{
	"seq": func(n int) []int {
		s := make([]int, n)
		for i := range s {
			s[i] = i
		}
		return s
	},
}

func New(fsys fs.FS) (*Engine, error) {
	base, err := fs.ReadFile(fsys, "templates/base.html")
	if err != nil {
		return nil, fmt.Errorf("read base.html: %w", err)
	}

	e := &Engine{templates: make(map[string]*template.Template)}

	for _, lang := range languages {
		for _, page := range pages {
			path := "templates/" + lang + "/" + page
			content, err := fs.ReadFile(fsys, path)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
			combined := string(base) + "\n" + string(content)
			key := lang + ":" + page
			t, err := template.New(key).Funcs(funcMap).Parse(combined)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", key, err)
			}
			e.templates[key] = t
		}
	}

	return e, nil
}

func (e *Engine) Render(w io.Writer, lang, page string, data any) error {
	key := lang + ":" + page
	t, ok := e.templates[key]
	if !ok {
		return fmt.Errorf("template not found: %s", key)
	}
	return t.ExecuteTemplate(w, "base", data)
}
