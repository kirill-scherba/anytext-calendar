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

// The styles and the script travel inside the page, not as files of their own. A module's frame is
// filled with srcdoc through the relay, and a <link> or a <script src> from that frame carries no
// account token: the relay refuses it, and the person sees bare HTML on a white background. Every
// module that works remotely is one self-contained document for this reason (21.11.2026).
var (
	pageStyle  = template.CSS(mustAsset("style.css"))
	pageScript = template.JS(mustAsset("app.js"))
)

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
		"Style":   pageStyle,
		"Script":  pageScript,
	})
}

// mustAsset reads one file of the screen at start: a missing file is a broken build, not a screen
// that half works.
func mustAsset(name string) string {
	data, err := uiFS.ReadFile("ui/" + name)
	if err != nil {
		panic("anytext-calendar: " + err.Error())
	}
	return string(data)
}
