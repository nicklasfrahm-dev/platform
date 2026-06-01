package ui

import (
	"embed"
	"html/template"
	"io/fs"
)

//go:embed templates
var templateFS embed.FS

func parseTemplates(page string) (*template.Template, error) {
	return template.New("").ParseFS(templateFS, "templates/layout.html", "templates/"+page)
}

func mustParse(page string) *template.Template {
	t, err := parseTemplates(page)
	if err != nil {
		panic("parse template " + page + ": " + err.Error())
	}
	return t
}

// TemplateFS exposes the embedded template filesystem for direct access if needed.
func TemplateFS() fs.FS { return templateFS }
