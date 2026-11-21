// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// recordedCall is one tools/call the fake gateway received.
type recordedCall struct {
	Server string
	Tool   string
	Args   map[string]any
}

// fakeGateway speaks the MCP gateway's HTTP dialect: POST /<server> with a JSON-RPC body.
type fakeGateway struct {
	mu     sync.Mutex
	calls  []recordedCall
	answer func(tool string, args map[string]any) (structured map[string]any, text string, isError bool, err string, status int)
}

func (g *fakeGateway) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		server := strings.Trim(strings.TrimPrefix(r.URL.Path, "/"), "/")
		g.mu.Lock()
		g.calls = append(g.calls, recordedCall{Server: server, Tool: req.Params.Name, Args: req.Params.Arguments})
		g.mu.Unlock()

		structured, text, isError, errText, status := g.answer(req.Params.Name, req.Params.Arguments)
		if status != 0 && status != http.StatusOK {
			http.Error(w, errText, status)
			return
		}
		result := map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
		if structured != nil {
			result["structuredContent"] = structured
		}
		if isError {
			result["isError"] = true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}
}

func (g *fakeGateway) lastCall(t *testing.T) recordedCall {
	t.Helper()
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.calls) == 0 {
		t.Fatal("the gateway received no call")
	}
	return g.calls[len(g.calls)-1]
}

func newFakeClient(t *testing.T, g *fakeGateway) *calendarClient {
	t.Helper()
	srv := httptest.NewServer(g.handler())
	t.Cleanup(srv.Close)
	return newCalendarClient(srv.URL, "mail-mcp")
}

// TestEventsReadStructuredContent: the module reads the typed answer, and an event without a calendar
// id gets the one it was asked for.
func TestEventsReadStructuredContent(t *testing.T) {
	g := &fakeGateway{answer: func(tool string, args map[string]any) (map[string]any, string, bool, string, int) {
		return map[string]any{"calendar_id": "primary", "events": []map[string]any{
			{"id": "e1", "summary": "Совещание", "start": "2026-11-21T10:00:00+03:00", "end": "2026-11-21T11:00:00+03:00"},
			{"id": "e2", "summary": "Отпуск", "start": "2026-11-22", "end": "2026-11-23", "all_day": true},
		}}, "ok", false, "", http.StatusOK
	}}
	c := newFakeClient(t, g)

	from := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	events, err := c.Events(context.Background(), "primary", from, from.AddDate(0, 0, 7))
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events", len(events))
	}
	if events[0].CalendarID != "primary" || events[1].CalendarID != "primary" {
		t.Errorf("calendar id not filled: %+v", events)
	}
	if !events[1].AllDay || events[1].Start != "2026-11-22" {
		t.Errorf("all-day event read wrong: %+v", events[1])
	}

	call := g.lastCall(t)
	if call.Server != "mail-mcp" || call.Tool != "calendar_list" {
		t.Errorf("call = %s/%s", call.Server, call.Tool)
	}
	if got := call.Args["max_results"]; got != float64(2500) {
		t.Errorf("max_results = %v", got)
	}
	if got := call.Args["calendar_id"]; got != "primary" {
		t.Errorf("calendar_id = %v", got)
	}
	for _, key := range []string{"time_min", "time_max"} {
		if _, err := time.Parse(time.RFC3339, call.Args[key].(string)); err != nil {
			t.Errorf("%s is not RFC 3339: %v", key, call.Args[key])
		}
	}
}

// TestOldServerWithoutStructuredContent: the error names the server and the version to install. A
// screen parsing Russian sentences out of a text answer is a screen that breaks on a wording change.
func TestOldServerWithoutStructuredContent(t *testing.T) {
	g := &fakeGateway{answer: func(tool string, args map[string]any) (map[string]any, string, bool, string, int) {
		return nil, "📅 События: 0", false, "", http.StatusOK
	}}
	c := newFakeClient(t, g)

	_, err := c.Events(context.Background(), "primary", time.Now(), time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("an answer without structuredContent was accepted")
	}
	if !strings.Contains(err.Error(), "mail-mcp до 1.1.1") {
		t.Errorf("error does not say what to do: %v", err)
	}
}

// TestToolErrorIsTheServersSentence: a refused call comes back as the server's own words.
func TestToolErrorIsTheServersSentence(t *testing.T) {
	g := &fakeGateway{answer: func(tool string, args map[string]any) (map[string]any, string, bool, string, int) {
		return nil, "Error: no token: run gmail_auth first", true, "", http.StatusOK
	}}
	c := newFakeClient(t, g)

	_, err := c.Calendars(context.Background(), true)
	if err == nil {
		t.Fatal("a refused call was accepted")
	}
	if !strings.Contains(err.Error(), "gmail_auth") {
		t.Errorf("error = %v", err)
	}
}

// TestCalendarsAreCached: a screen refreshing its events must not ask Google for the calendar list
// every second; ?fresh=1 is the way to ask anyway.
func TestCalendarsAreCached(t *testing.T) {
	g := &fakeGateway{answer: func(tool string, args map[string]any) (map[string]any, string, bool, string, int) {
		return map[string]any{"calendars": []map[string]any{
			{"id": "primary", "summary": "Кирилл", "primary": true, "background_color": "#d06b64"},
		}}, "ok", false, "", http.StatusOK
	}}
	c := newFakeClient(t, g)

	if _, err := c.Calendars(context.Background(), false); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := c.Calendars(context.Background(), false); err != nil {
		t.Fatalf("second call: %v", err)
	}
	g.mu.Lock()
	calls := len(g.calls)
	g.mu.Unlock()
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (the second answer must come from the cache)", calls)
	}

	if _, err := c.Calendars(context.Background(), true); err != nil {
		t.Fatalf("fresh call: %v", err)
	}
	g.mu.Lock()
	calls = len(g.calls)
	g.mu.Unlock()
	if calls != 2 {
		t.Errorf("calls after fresh = %d, want 2", calls)
	}
}

// TestCreatePassesTheEditorsFieldsThrough: the tool gets exactly what a person filled in.
func TestCreatePassesTheEditorsFieldsThrough(t *testing.T) {
	g := &fakeGateway{answer: func(tool string, args map[string]any) (map[string]any, string, bool, string, int) {
		if tool != "calendar_create" {
			t.Errorf("tool = %s", tool)
		}
		if args["summary"] != "Встреча" {
			t.Errorf("summary = %v", args["summary"])
		}
		if got, ok := args["attendees"].([]any); !ok || len(got) != 1 || got[0] != "a@example.com" {
			t.Errorf("attendees = %v", args["attendees"])
		}
		return map[string]any{"event": map[string]any{
			"id": "new1", "summary": "Встреча", "start": args["start"], "end": args["end"],
		}}, "ok", false, "", http.StatusOK
	}}
	c := newFakeClient(t, g)

	summary := "Встреча"
	start := "2026-11-21T10:00:00+03:00"
	end := "2026-11-21T11:00:00+03:00"
	location := "Кабинет"
	att := []string{"a@example.com"}
	args, err := createArgs(eventInput{
		CalendarID: "team", Summary: &summary, Start: &start, End: &end, Location: &location, Attendees: &att,
	})
	if err != nil {
		t.Fatalf("createArgs: %v", err)
	}
	ev, err := c.Create(context.Background(), args)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ev.ID != "new1" {
		t.Errorf("event = %+v", ev)
	}
	if got := g.lastCall(t).Args["calendar_id"]; got != "team" {
		t.Errorf("calendar_id = %v", got)
	}
	if got := g.lastCall(t).Args["location"]; got != "Кабинет" {
		t.Errorf("location = %v", got)
	}
}

// TestUpdateSendsOnlyTouchedFields: a partial update must not rewrite what a person did not open.
func TestUpdateSendsOnlyTouchedFields(t *testing.T) {
	summary := "Новое имя"
	args, err := updateArgs("primary", "e1", eventInput{Summary: &summary})
	if err != nil {
		t.Fatalf("updateArgs: %v", err)
	}
	if len(args) != 3 {
		t.Errorf("args = %v, want calendar_id, event_id and summary", args)
	}
	if args["summary"] != "Новое имя" {
		t.Errorf("summary = %v", args["summary"])
	}

	empty := ""
	args, err = updateArgs("primary", "e1", eventInput{Description: &empty})
	if err != nil {
		t.Fatalf("updateArgs: %v", err)
	}
	if got, ok := args["description"]; !ok || got != "" {
		t.Errorf("an erased description must be sent as empty, got %v", args)
	}

	none := []string{}
	args, err = updateArgs("primary", "e1", eventInput{Attendees: &none})
	if err != nil {
		t.Fatalf("updateArgs: %v", err)
	}
	if got, ok := args["attendees"].([]string); !ok || len(got) != 0 {
		t.Errorf("an emptied attendee list must be sent as empty, got %v", args["attendees"])
	}
}

// TestUpdateRefusesHalfATimeChange: an event moved without its end has no length.
func TestUpdateRefusesHalfATimeChange(t *testing.T) {
	start := "2026-11-21T10:00:00+03:00"
	if _, err := updateArgs("primary", "e1", eventInput{Start: &start}); err == nil {
		t.Fatal("a start without an end was accepted")
	}
}

// TestCheckTimes: the shapes are told apart before Google has to answer.
func TestCheckTimes(t *testing.T) {
	if err := checkTimes("2026-11-21", "2026-11-22", true); err != nil {
		t.Errorf("dates for an all-day event: %v", err)
	}
	if err := checkTimes("2026-11-21T10:00:00+03:00", "2026-11-21", true); err == nil {
		t.Error("a timestamp start with a date end was accepted for an all-day event")
	}
	if err := checkTimes("2026-11-21T10:00:00+03:00", "2026-11-21T11:00:00+03:00", false); err != nil {
		t.Errorf("timestamps: %v", err)
	}
	if err := checkTimes("2026-11-21T11:00:00+03:00", "2026-11-21T10:00:00+03:00", false); err == nil {
		t.Error("an end before the start was accepted")
	}
	if err := checkTimes("2026-11-21T10:00:00", "2026-11-21T11:00:00", false); err == nil {
		t.Error("a timestamp without an offset was accepted")
	}
}
