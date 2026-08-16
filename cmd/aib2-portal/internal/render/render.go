package render

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
)

type Engine struct {
	templates map[string]*template.Template // "en:home.html" -> compiled template
	languages []string // Supported languages
}

// Core languages with full translations
var coreLanguages = []string{"en", "zh", "zh-tw", "fr", "ja", "ms", "th", "es", "de", "ru"}

// Fallback languages for AI agent extension
// Agents can add new languages by creating template directories and calling RegisterLanguage
var extendedLanguages = []string{}

var pages = []string{
	"landing.html",
	"home.html",
	"constitution.html",
	"poaiw.html",
	"plan.html",
	"genesis.html",
	"roadmap.html",
	"standards.html",
	"changelog.html",
	"ai-context.html",
	"explorer.html",
	"quickstart.html",
	"wallet.html",
	"aib2-home.html",
	"aib2-about.html",
	"aib2-docs.html",
	"aib2-team.html",
	"aib2-history.html",
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

	e := &Engine{
		templates: make(map[string]*template.Template),
		languages: append(coreLanguages, extendedLanguages...),
	}

	for _, lang := range e.languages {
		for _, page := range pages {
			path := "templates/" + lang + "/" + page
			content, err := fs.ReadFile(fsys, path)

			// If template doesn't exist, fall back to English
			if err != nil && lang != "en" {
				path = "templates/en/" + page
				content, err = fs.ReadFile(fsys, path)
			}

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

// RegisterLanguage allows AI agents to dynamically add new languages
// Call this with the language code (e.g., "ko", "vi", "ar") after creating the engine
func (e *Engine) RegisterLanguage(fsys fs.FS, lang string) error {
	// Check if already registered
	for _, l := range e.languages {
		if l == lang {
			return nil // Already registered
		}
	}

	e.languages = append(e.languages, lang)

	// Load templates for new language
	base, err := fs.ReadFile(fsys, "templates/base.html")
	if err != nil {
		return fmt.Errorf("read base.html: %w", err)
	}

	for _, page := range pages {
		path := "templates/" + lang + "/" + page
		content, err := fs.ReadFile(fsys, path)

		// Fall back to English if template doesn't exist
		if err != nil {
			path = "templates/en/" + page
			content, err = fs.ReadFile(fsys, path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
		}

		combined := string(base) + "\n" + string(content)
		key := lang + ":" + page
		t, err := template.New(key).Funcs(funcMap).Parse(combined)
		if err != nil {
			return fmt.Errorf("parse %s: %w", key, err)
		}
		e.templates[key] = t
	}

	return nil
}

// GetLanguages returns all supported languages
func (e *Engine) GetLanguages() []string {
	return e.languages
}

func (e *Engine) Render(w io.Writer, lang, page string, data any) error {
	key := lang + ":" + page
	t, ok := e.templates[key]
	if !ok {
		return fmt.Errorf("template not found: %s", key)
	}
	return t.ExecuteTemplate(w, "base", data)
}
