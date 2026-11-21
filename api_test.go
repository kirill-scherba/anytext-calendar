// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testApp is the module's HTTP surface over a fake gateway.
func testApp(t *testing.T, g *fakeGateway) *httptest.Server {
	t.Helper()
	gateway := httptest.NewServer(g.handler())
	t.Cleanup(gateway.Close)
	a := &app{cal: newCalendarClient(gateway.URL, "mail-mcp")}
	srv := httptest.NewServer(a.routes())
	t.Cleanup(srv.Close)
	return srv
}

// calendarGateway answers the calendar tools: two calendars, events only in the primary one, and one
// calendar that fails - the case of a shared calendar whose owner revoked access.
func calendarGateway(failing string) *fakeGateway {
	return &fakeGateway{answer: func(tool string, args map[string]any) (map[string]any, string, bool, string, int) {
		switch tool {
		case "calendar_calendars":
			return map[string]any{"calendars": []map[string]any{
				{"id": "primary", "summary": "Дом", "primary": true, "selected": true, "background_color": "#d06b64"},
				{"id": "team", "summary": "Работа", "selected": true, "background_color": "#33b679"},
			}}, "ok", false, "", http.StatusOK
		case "calendar_list":
			id, _ := args["calendar_id"].(string)
			if id == failing {
				return nil, "Error: forbidden", true, "", http.StatusOK
			}
			if id == "team" {
				return map[string]any{"calendar_id": "team", "events": []map[string]any{
					{"id": "t1", "summary": "Планёрка", "start": "2026-11-23T10:00:00+03:00", "end": "2026-11-23T11:00:00+03:00"},
				}}, "ok", false, "", http.StatusOK
			}
			return map[string]any{"calendar_id": "primary", "events": []map[string]any{
				{"id": "p1", "summary": "Обед", "start": "2026-11-21T13:00:00+03:00", "end": "2026-11-21T14:00:00+03:00"},
				{"id": "p2", "summary": "Отпуск", "start": "2026-11-22", "end": "2026-11-23", "all_day": true},
			}}, "ok", false, "", http.StatusOK
		case "calendar_create", "calendar_update":
			event := map[string]any{"id": "made", "summary": args["summary"], "start": args["start"], "end": args["end"]}
			return map[string]any{"calendar_id": args["calendar_id"], "event": event}, "ok", false, "", http.StatusOK
		case "calendar_delete":
			return map[string]any{"calendar_id": args["calendar_id"], "event_id": args["event_id"], "deleted": true}, "ok", false, "", http.StatusOK
		}
		return nil, "unknown tool", true, "", http.StatusOK
	}}
}

// TestEventsMergeCalendars: two calendars in one answer, all-day events first, each event knowing its
// calendar's name and colour.
func TestEventsMergeCalendars(t *testing.T) {
	srv := testApp(t, calendarGateway(""))

	res, err := http.Get(srv.URL + "/api/events?from=2026-11-21T00:00:00%2B03:00&to=2026-11-24T00:00:00%2B03:00&calendars=primary,team")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var answer struct {
		Events []shownEvent `json:"events"`
		Count  int          `json:"count"`
		Errors []struct {
			CalendarID string `json:"calendar_id"`
			Error      string `json:"error"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(res.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if answer.Count != 3 || len(answer.Events) != 3 {
		t.Fatalf("count = %d, events = %d", answer.Count, len(answer.Events))
	}
	if !answer.Events[0].AllDay || answer.Events[0].Summary != "Отпуск" {
		t.Errorf("first event is not the all-day one: %+v", answer.Events[0])
	}
	seen := map[string]bool{}
	for _, ev := range answer.Events {
		seen[ev.CalendarName] = true
		if ev.CalendarColor == "" {
			t.Errorf("event %s has no colour", ev.ID)
		}
	}
	if !seen["Дом"] || !seen["Работа"] {
		t.Errorf("calendar names missing: %v", seen)
	}
	if len(answer.Errors) != 0 {
		t.Errorf("errors = %+v", answer.Errors)
	}
}

// TestEventsReportOneBrokenCalendar: a calendar that refuses is named, and the other one is still
// shown - a person with a broken shared calendar must not lose his own.
func TestEventsReportOneBrokenCalendar(t *testing.T) {
	srv := testApp(t, calendarGateway("team"))

	res, err := http.Get(srv.URL + "/api/events?from=2026-11-21T00:00:00%2B03:00&to=2026-11-24T00:00:00%2B03:00&calendars=primary,team")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var answer struct {
		Events []shownEvent `json:"events"`
		Errors []struct {
			CalendarID string `json:"calendar_id"`
			Summary    string `json:"summary"`
			Error      string `json:"error"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(res.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Events) != 2 {
		t.Errorf("events = %d, want the two of the primary calendar", len(answer.Events))
	}
	if len(answer.Errors) != 1 || answer.Errors[0].CalendarID != "team" || answer.Errors[0].Summary != "Работа" {
		t.Fatalf("errors = %+v", answer.Errors)
	}
	if !strings.Contains(answer.Errors[0].Error, "forbidden") {
		t.Errorf("error = %q", answer.Errors[0].Error)
	}
}

// TestEventsNeedARange: a screen that forgot its window gets a sentence, not a guess.
func TestEventsNeedARange(t *testing.T) {
	srv := testApp(t, calendarGateway(""))
	res, err := http.Get(srv.URL + "/api/events?calendars=primary")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.StatusCode)
	}
}

// TestCreateAllDaySendsDates: an all-day event goes to the tool as a date pair.
func TestCreateAllDaySendsDates(t *testing.T) {
	g := calendarGateway("")
	srv := testApp(t, g)

	body := `{"calendar_id":"primary","summary":"Отпуск","start":"2026-12-01","end":"2026-12-03","all_day":true,"description":"","location":"","attendees":[]}`
	res, err := http.Post(srv.URL+"/api/events", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	call := g.lastCall(t)
	if call.Tool != "calendar_create" {
		t.Fatalf("tool = %s", call.Tool)
	}
	if call.Args["start"] != "2026-12-01" || call.Args["end"] != "2026-12-03" {
		t.Errorf("start/end = %v/%v", call.Args["start"], call.Args["end"])
	}
	if call.Args["summary"] != "Отпуск" {
		t.Errorf("summary = %v", call.Args["summary"])
	}
}

// TestCreateRefusesWithoutTitle: no title, no event.
func TestCreateRefusesWithoutTitle(t *testing.T) {
	srv := testApp(t, calendarGateway(""))
	body := `{"summary":"   ","start":"2026-12-01","end":"2026-12-01","all_day":true}`
	res, err := http.Post(srv.URL+"/api/events", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.StatusCode)
	}
}

// TestUpdateAndDelete: the editor's two write paths reach the tools with the event's calendar.
func TestUpdateAndDelete(t *testing.T) {
	g := calendarGateway("")
	srv := testApp(t, g)

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/events?id=e1&calendar=team",
		strings.NewReader(`{"summary":"Новое имя","start":"2026-11-21T10:00:00+03:00","end":"2026-11-21T11:00:00+03:00","all_day":false}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("patch status = %d", res.StatusCode)
	}
	call := g.lastCall(t)
	if call.Tool != "calendar_update" || call.Args["event_id"] != "e1" || call.Args["calendar_id"] != "team" {
		t.Errorf("update call = %+v", call)
	}

	req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/api/events?id=e1&calendar=team", nil)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", res.StatusCode)
	}
	call = g.lastCall(t)
	if call.Tool != "calendar_delete" || call.Args["event_id"] != "e1" {
		t.Errorf("delete call = %+v", call)
	}
}

// TestStatusSaysNotReady: the screen must be able to tell "nothing planned" from "the gateway is off".
func TestStatusSaysNotReady(t *testing.T) {
	g := &fakeGateway{answer: func(tool string, args map[string]any) (map[string]any, string, bool, string, int) {
		return nil, "", false, "MCP gateway is off", http.StatusBadGateway
	}}
	srv := testApp(t, g)

	res, err := http.Get(srv.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var answer struct {
		Ready bool   `json:"ready"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if answer.Ready {
		t.Error("ready = true while the gateway refuses")
	}
	if answer.Error == "" {
		t.Error("no error text for the screen")
	}
}

// TestPageCarriesTheBase: the screen's own requests must land on the module root, both at the machine
// and behind the relay.
func TestPageCarriesTheBase(t *testing.T) {
	srv := testApp(t, calendarGateway(""))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/ui/", nil)
	req.Header.Set("X-AnyText-Prefix", "/s/dev/api/module/calendar")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	if !strings.Contains(html, `href="/s/dev/api/module/calendar/"`) {
		t.Errorf("page has no base from the installation: %s", firstLine(html))
	}
	if strings.Contains(html, "{{") {
		t.Error("template markers left in the page")
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// TestEventsResolveThePrimaryAlias: Google calls the main calendar "primary" and lists it by its
// address. An event asked for as "primary" must reach the screen with the address as its calendar id,
// otherwise it has no name and no colour, and the wrong id goes back on save.
func TestEventsResolveThePrimaryAlias(t *testing.T) {
	g := &fakeGateway{answer: func(tool string, args map[string]any) (map[string]any, string, bool, string, int) {
		switch tool {
		case "calendar_calendars":
			return map[string]any{"calendars": []map[string]any{
				{"id": "me@example.com", "summary": "Дом", "primary": true, "background_color": "#d06b64"},
			}}, "ok", false, "", http.StatusOK
		case "calendar_list":
			return map[string]any{"calendar_id": args["calendar_id"], "events": []map[string]any{
				{"id": "e1", "summary": "Обед", "start": "2026-11-21T13:00:00+03:00", "end": "2026-11-21T14:00:00+03:00"},
			}}, "ok", false, "", http.StatusOK
		}
		return nil, "unknown tool", true, "", http.StatusOK
	}}
	srv := testApp(t, g)

	res, err := http.Get(srv.URL + "/api/events?from=2026-11-21T00:00:00%2B03:00&to=2026-11-22T00:00:00%2B03:00&calendars=primary")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var answer struct {
		Events []shownEvent `json:"events"`
	}
	if err := json.NewDecoder(res.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Events) != 1 {
		t.Fatalf("events = %d", len(answer.Events))
	}
	ev := answer.Events[0]
	if ev.CalendarID != "me@example.com" || ev.CalendarName != "Дом" || ev.CalendarColor != "#d06b64" {
		t.Errorf("event not resolved to the real calendar: %+v", ev)
	}
}
