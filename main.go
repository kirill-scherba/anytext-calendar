// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command anytext-calendar is the calendar module of AnyText: a screen for a person over the calendar
// that mail-mcp already owns (docs/MODULES.md in anytext-gui).
//
// The module is a process with its own HTTP server on loopback. The installation starts it, proxies
// /api/module/calendar/... to it, tells it who is asking and where the MCP gateway answers; the
// calendar data itself comes from mail-mcp through that gateway. No Google token and no OAuth code
// lives here: one place that talks to Google is enough to keep working (21.11.2026).
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
)

// version is the module's own version: it is shown in the screen and makes "which module is this"
// answerable on a machine with two installations.
const version = "0.1.1"

func main() {
	addr := flag.String("addr", "127.0.0.1:7840", "address to listen on (loopback only)")
	mcp := flag.String("mcp", "", "MCP gateway base address (default: ANYTEXT_MCP_PROXY or http://127.0.0.1:7711)")
	server := flag.String("server", "mail-mcp", "MCP server that holds the calendar tools")
	flag.Parse()

	base := strings.TrimRight(strings.TrimSpace(*mcp), "/")
	if base == "" {
		base = strings.TrimRight(os.Getenv("ANYTEXT_MCP_PROXY"), "/")
	}
	if base == "" {
		base = "http://127.0.0.1:7711"
	}

	a := &app{cal: newCalendarClient(base, *server)}
	log.Printf("anytext-calendar %s: %s, gateway %s, server %s", version, *addr, base, *server)
	log.Fatal(http.ListenAndServe(*addr, a.routes()))
}
