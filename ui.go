// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// ui.go — the module's screens. They are plain files in ui/, served with a base address, so the same
// page works on this machine and through the relay.
package main

import (
	"embed"
	"html/template"
	"net/http"
	"strings"
)

//go:embed ui/index.html ui/app.js ui/style.css
var uiFS embed.FS

// pageTemplate is index.html with the one thing the server knows: where the module is mounted.
var pageTemplate = template.Must(template.ParseFS(uiFS, "ui/index.html"))

// page serves the screen. The prefix comes from the installation ("/api/module/calendar"), and a
// <base href> built from it makes the page's own requests land on the module's root: relative paths
// then work the same at the machine and through the relay.
func (a *app) page(w http.ResponseWriter, r *http.Request) {
	prefix := r.Header.Get("X-AnyText-Prefix")
	if prefix == "" {
		prefix = "/api/module/calendar"
	}
	person := r.Header.Get("X-AnyText-Person")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = pageTemplate.Execute(w, map[string]any{
		"Prefix":  strings.TrimRight(prefix, "/"),
		"Person":  person,
		"Version": version,
	})
}

// asset serves one file of the screen. A module is updated by replacing the binary, so nothing here
// may be cached by a browser: an old app.js against a new API is a screen that half works.
func (a *app) asset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := uiFS.ReadFile("ui/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	}
}
