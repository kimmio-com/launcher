package launcher

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"sync"
)

type Templates struct {
	t     *template.Template
	pages map[string]struct{}
	mu    sync.RWMutex
}

func NewTemplatesFromFS(fsys fs.FS, root string) (*Templates, error) {
	var files []string
	pages := map[string]struct{}{}

	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if path.Ext(p) != ".html" {
			return nil
		}
		files = append(files, p)

		if path.Dir(p) == root {
			pages[path.Base(p)] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk templates: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no templates found under %q", root)
	}

	t, err := template.ParseFS(fsys, files...)
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	return &Templates{t: t, pages: pages}, nil
}

func (ts *Templates) HasPage(pageName string) bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	_, ok := ts.pages[pageName]
	return ok
}

func (ts *Templates) RenderPageWithTemplate(w http.ResponseWriter, pageName string, data map[string]any) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	ts.mu.RLock()
	base := ts.t
	_, ok := ts.pages[pageName]
	ts.mu.RUnlock()

	if !ok {
		return fmt.Errorf("page not found in templates: %s", pageName)
	}

	clone, err := base.Clone()
	if err != nil {
		return err
	}

	pageTpl := "page:" + pageName

	// IMPORTANT: redefine the existing template "page" (overwrite in the clone)
	_, err = clone.Parse(`{{ define "page" }}{{ template "` + pageTpl + `" . }}{{ end }}`)
	if err != nil {
		return fmt.Errorf("define page alias: %w", err)
	}

	return clone.ExecuteTemplate(w, "layout", data)
}
