// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// api.go — the module's own HTTP API: what its screen asks and what it changes.
//
// Every call here is made by the screen through the installation's proxy, which has already checked
// who is asking (docs/MODULES.md in anytext-gui). The module itself listens on loopback and has no
// authentication of its own, on purpose: the installation is the only door.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// app is the module with its two dependencies: the calendar that mail-mcp owns, and the
// installation's clock, which owns the calendar watcher.
type app struct {
	cal   *calendarClient
	clock *schedulerClient
}

// routes is the whole surface of the module.
func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /api/status", a.status)
	mux.HandleFunc("GET /api/calendars", a.calendarList)
	mux.HandleFunc("GET /api/events", a.events)
	mux.HandleFunc("POST /api/events", a.createEvent)
	mux.HandleFunc("PATCH /api/events", a.updateEvent)
	mux.HandleFunc("DELETE /api/events", a.deleteEvent)
	mux.HandleFunc("GET /api/reminders", a.reminders)
	mux.HandleFunc("POST /api/reminders", a.setReminders)
	// The screen is one document: its styles and its script come inside it (ui.go explains why).
	mux.HandleFunc("GET /ui/", a.page)
	return mux
}

// status says whether the calendar is reachable at all: the screen must be able to tell "nothing
// planned" from "the gateway is off".
func (a *app) status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	list, err := a.cal.Calendars(ctx, false)
	answer := map[string]any{
		"version":   version,
		"gateway":   a.cal.base,
		"server":    a.cal.server,
		"ready":     err == nil,
		"calendars": len(list),
	}
	if err != nil {
		answer["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, answer)
}

// calendarList answers with the calendars a person may show. ?fresh=1 goes to Google again - a
// calendar added in Google a minute ago should not wait for the cache.
func (a *app) calendarList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	list, err := a.cal.Calendars(ctx, r.URL.Query().Get("fresh") == "1")
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"calendars": list})
}

// events answers with everything in a range, one call per calendar. A calendar that fails is named
// in errors and does not blank the screen: a person with a broken shared calendar still sees his own.
func (a *app) events(w http.ResponseWriter, r *http.Request) {
	from, to, err := rangeOf(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ids := splitList(r.URL.Query().Get("calendars"))
	if len(ids) == 0 {
		ids = []string{"primary"}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	known, _ := a.cal.Calendars(ctx, false)
	byID := make(map[string]Calendar, len(known))
	var primaryID string
	for _, cal := range known {
		byID[cal.ID] = cal
		if cal.Primary {
			primaryID = cal.ID
		}
	}
	// resolve finds the calendar a screen named. "primary" is Google's own alias for the person's
	// main calendar, and the calendar list calls it by its address: without this the events of the
	// primary calendar had no name and no colour on the screen (21.11.2026).
	resolve := func(id string) (Calendar, bool) {
		if cal, ok := byID[id]; ok {
			return cal, true
		}
		if id == "primary" && primaryID != "" {
			return byID[primaryID], true
		}
		return Calendar{}, false
	}

	type calError struct {
		CalendarID string `json:"calendar_id"`
		Summary    string `json:"summary,omitempty"`
		Error      string `json:"error"`
	}
	var (
		mu     sync.Mutex
		events []shownEvent
		errs   []calError
		wg     sync.WaitGroup
	)
	// Four calendars at a time: enough to be quick, few enough not to be a burst at Google from one
	// screen.
	sem := make(chan struct{}, 4)
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			list, err := a.cal.Events(ctx, id, from, to)
			cal, hasCal := resolve(id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				e := calError{CalendarID: id, Error: err.Error()}
				if hasCal {
					e.Summary = cal.Summary
				}
				errs = append(errs, e)
				return
			}
			for _, ev := range list {
				shown := shownEvent{Event: ev}
				if hasCal {
					// The event answers with the id it was asked for; the screen knows calendars by
					// their real ids, so the alias is replaced here once.
					shown.CalendarID = cal.ID
					shown.CalendarName = cal.Summary
					shown.CalendarColor = cal.BackgroundColor
				}
				events = append(events, shown)
			}
		}(id)
	}
	wg.Wait()

	SortEvents(events)
	if events == nil {
		events = []shownEvent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from":   from.Format(time.RFC3339),
		"to":     to.Format(time.RFC3339),
		"count":  len(events),
		"events": events,
		"errors": errs,
	})
}

// eventInput is what the editor sends. Pointers tell "not sent" from "sent empty": an erased
// description is erased, not left alone.
type eventInput struct {
	CalendarID  string    `json:"calendar_id"`
	Summary     *string   `json:"summary"`
	Start       *string   `json:"start"`
	End         *string   `json:"end"`
	AllDay      *bool     `json:"all_day"`
	Description *string   `json:"description"`
	Location    *string   `json:"location"`
	Attendees   *[]string `json:"attendees"`
}

// createEvent makes an event in the chosen calendar.
func (a *app) createEvent(w http.ResponseWriter, r *http.Request) {
	var in eventInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("тело запроса не разобрано: %w", err))
		return
	}
	if in.Summary == nil || strings.TrimSpace(*in.Summary) == "" {
		writeError(w, http.StatusBadRequest, errors.New("нужно название события"))
		return
	}
	args, err := createArgs(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	ev, err := a.cal.Create(ctx, args)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"event": ev})
}

// updateEvent changes the fields a person edited.
func (a *app) updateEvent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("нужен id события"))
		return
	}
	calendarID := q.Get("calendar")
	if calendarID == "" {
		calendarID = "primary"
	}
	var in eventInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("тело запроса не разобрано: %w", err))
		return
	}
	args, err := updateArgs(calendarID, id, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	ev, err := a.cal.Update(ctx, args)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"event": ev})
}

// deleteEvent removes one event, or one occurrence of a recurring one.
func (a *app) deleteEvent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("нужен id события"))
		return
	}
	calendarID := q.Get("calendar")
	if calendarID == "" {
		calendarID = "primary"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := a.cal.Delete(ctx, calendarID, id); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

// rangeOf reads from and to: RFC 3339 timestamps with an offset, as a browser can write them.
func rangeOf(r *http.Request) (time.Time, time.Time, error) {
	q := r.URL.Query()
	parse := func(name string) (time.Time, error) {
		raw := q.Get(name)
		if raw == "" {
			return time.Time{}, fmt.Errorf("нужен параметр %s", name)
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s: нужен RFC 3339 со смещением", name)
		}
		return t, nil
	}
	from, err := parse("from")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to, err := parse("to")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, errors.New("from должен быть раньше to")
	}
	if to.Sub(from) > 400*24*time.Hour {
		return time.Time{}, time.Time{}, errors.New("слишком длинный промежуток: не больше года")
	}
	return from, to, nil
}

// createArgs turns the editor's fields into the arguments of calendar_create.
func createArgs(in eventInput) (map[string]any, error) {
	if in.Start == nil || in.End == nil {
		return nil, errors.New("нужны начало и конец события")
	}
	allDay := in.AllDay != nil && *in.AllDay
	if err := checkTimes(*in.Start, *in.End, allDay); err != nil {
		return nil, err
	}
	args := map[string]any{
		"summary": strings.TrimSpace(*in.Summary),
		"start":   *in.Start,
		"end":     *in.End,
	}
	if in.CalendarID != "" {
		args["calendar_id"] = in.CalendarID
	}
	if in.Description != nil {
		args["description"] = *in.Description
	}
	if in.Location != nil {
		args["location"] = *in.Location
	}
	if in.Attendees != nil {
		args["attendees"] = *in.Attendees
	}
	return args, nil
}

// updateArgs turns the editor into a partial update: only the fields a person touched.
func updateArgs(calendarID, eventID string, in eventInput) (map[string]any, error) {
	args := map[string]any{"calendar_id": calendarID, "event_id": eventID}
	if in.Summary != nil {
		args["summary"] = strings.TrimSpace(*in.Summary)
	}
	if in.Description != nil {
		args["description"] = *in.Description
	}
	if in.Location != nil {
		args["location"] = *in.Location
	}
	if in.Attendees != nil {
		args["attendees"] = *in.Attendees
	}
	if in.Start != nil || in.End != nil {
		if in.Start == nil || in.End == nil {
			// Moving an event means both its ends: calendar_update has no "same length" idea.
			return nil, errors.New("для переноса события нужны и начало, и конец")
		}
		allDay := in.AllDay != nil && *in.AllDay
		if err := checkTimes(*in.Start, *in.End, allDay); err != nil {
			return nil, err
		}
		args["start"], args["end"] = *in.Start, *in.End
	}
	if len(args) == 2 {
		return nil, errors.New("нечего менять")
	}
	return args, nil
}

// checkTimes checks the shape of a start/end pair: dates for an all-day event, RFC 3339 timestamps
// with an offset otherwise. The calendar itself would answer with a Google error; a sentence in the
// person's language is more useful.
func checkTimes(start, end string, allDay bool) error {
	if allDay {
		s, err := time.Parse("2006-01-02", start)
		if err != nil {
			return errors.New("начало события на весь день — дата вида 2026-11-21")
		}
		e, err := time.Parse("2006-01-02", end)
		if err != nil {
			return errors.New("конец события на весь день — дата вида 2026-11-21")
		}
		if e.Before(s) {
			return errors.New("конец раньше начала")
		}
		return nil
	}
	s, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return errors.New("начало: время в формате RFC 3339 со смещением")
	}
	e, err := time.Parse(time.RFC3339, end)
	if err != nil {
		return errors.New("конец: время в формате RFC 3339 со смещением")
	}
	if !s.Before(e) {
		return errors.New("конец должен быть позже начала")
	}
	return nil
}

// splitList reads a comma-separated list, dropping empty pieces: a calendar id may contain commas
// (an address like ru.russian#holiday@group.v.calendar.google.com does not, but a group id might),
// so the client joins with commas and this function keeps every non-empty id as it is.
func splitList(raw string) []string {
	out := make([]string, 0, 4)
	for _, piece := range strings.Split(raw, ",") {
		if piece = strings.TrimSpace(piece); piece != "" {
			out = append(out, piece)
		}
	}
	return out
}

// sortCalendars orders the list the way the screen shows it: the primary calendar first.
func sortCalendars(list []Calendar) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Primary != list[j].Primary {
			return list[i].Primary
		}
		return list[i].Summary < list[j].Summary
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]any{"error": err.Error()})
}

// reminders says whether the calendar watcher runs. The task itself lives in the installation's
// clock and is visible in the tasks module; this module only owns its on/off switch and its text.
func (a *app) reminders(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if a.clock == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "error": "установка не сообщила свой адрес"})
		return
	}
	task, found, err := a.clock.Watcher(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "error": err.Error()})
		return
	}
	answer := map[string]any{"enabled": found && task.Enabled, "lead_min": watchLead, "every": watchEvery}
	if found {
		answer["task_id"] = task.ID
	}
	writeJSON(w, http.StatusOK, answer)
}

// setReminders turns the watcher on or off. On creates it when it is missing and rewrites its text
// when it is there: the text belongs to the module, the clock belongs to the installation.
func (a *app) setReminders(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool   `json:"enabled"`
		Lang    string `json:"lang"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("тело запроса не разобрано: %w", err))
		return
	}
	if a.clock == nil {
		writeError(w, http.StatusBadGateway, errors.New("установка не сообщила свой адрес: напоминания недоступны"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	task, found, err := a.clock.Watcher(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if !found {
		if !in.Enabled {
			writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
			return
		}
		created, err := a.clock.Create(ctx, watcherTitle(langOf(in.Lang, r)), watchAction(langOf(in.Lang, r)),
			time.Now().Add(time.Minute), watchEvery)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "task_id": created.ID})
		return
	}
	// The text is refreshed every time the switch is used: the calendars and the lead live in the
	// module, and a task edited by hand in the tasks module gets the current text back only when a
	// person touches this switch on purpose.
	if in.Enabled {
		lang := langOf(in.Lang, r)
		if err := a.clock.SetTexts(ctx, task.ID, watcherTitle(lang), watchAction(lang)); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	if err := a.clock.Toggle(ctx, task.ID, in.Enabled); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": in.Enabled, "task_id": task.ID})
}

// langOf is the language of the window: what the screen said, or what the browser asks for. The task
// text and its name are written in it, so a person working in English does not get a Russian task
// (21.11.2026).
func langOf(fromScreen string, r *http.Request) string {
	candidates := []string{fromScreen, r.Header.Get("X-AnyText-Lang"), r.Header.Get("Accept-Language")}
	for _, raw := range candidates {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "en") {
			return "en"
		}
	}
	return "ru"
}
