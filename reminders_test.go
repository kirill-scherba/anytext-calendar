// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeClock is an installation whose schedule answers the four tools this module uses.
type fakeClock struct {
	mu      sync.Mutex
	tasks   []map[string]any
	created []map[string]any
	toggled []bool
	updated []string
	titles  []string
}

func (f *fakeClock) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		defer f.mu.Unlock()
		var result any
		switch req.Params.Name {
		case "scheduler_list":
			result = map[string]any{"tasks": f.tasks, "runs": []any{}}
		case "scheduler_create":
			f.created = append(f.created, req.Params.Arguments)
			result = map[string]any{"id": "made", "title": req.Params.Arguments["title"], "enabled": true}
		case "scheduler_update":
			if a, ok := req.Params.Arguments["action"].(string); ok {
				f.updated = append(f.updated, a)
			}
			if t, ok := req.Params.Arguments["title"].(string); ok {
				f.titles = append(f.titles, t)
			}
			result = map[string]any{"id": req.Params.Arguments["id"], "enabled": true}
		case "scheduler_toggle":
			f.toggled = append(f.toggled, req.Params.Arguments["enabled"] == true)
			result = map[string]any{"ok": true}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": req.ID,
			"result": map[string]any{
				"content":           []map[string]any{{"type": "text", "text": "ok"}},
				"structuredContent": result,
			},
		})
	}
}

func remindersServer(t *testing.T, clock *fakeClock) *httptest.Server {
	t.Helper()
	install := httptest.NewServer(clock.handler())
	t.Cleanup(install.Close)
	a := &app{clock: newSchedulerClient(install.URL)}
	srv := httptest.NewServer(a.routes())
	t.Cleanup(srv.Close)
	return srv
}

// TestRemindersSwitchCreatesTheWatcher: with no task, the switch on creates one, and the text names
// the calendars and the lead - the delivery is not mentioned, because the installation decides it.
func TestRemindersSwitchCreatesTheWatcher(t *testing.T) {
	clock := &fakeClock{}
	srv := remindersServer(t, clock)

	res, err := http.Post(srv.URL+"/api/reminders", "application/json", strings.NewReader(`{"enabled":true}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if len(clock.created) != 1 {
		t.Fatalf("created = %d", len(clock.created))
	}
	action, _ := clock.created[0]["action"].(string)
	for _, want := range []string{"notify_send", "kiriushka@gmail.com", "30 минут"} {
		if !strings.Contains(action, want) {
			t.Errorf("the task text does not mention %q", want)
		}
	}
	if strings.Contains(action, "notify_opencode") {
		t.Error("the task still talks to the chat directly: the installation owns the channels")
	}
	if got := clock.created[0]["every"]; got != "15m" {
		t.Errorf("every = %v", got)
	}
	if title, _ := clock.created[0]["title"].(string); !strings.HasPrefix(title, "Сторож календаря") {
		t.Errorf("a Russian window must name the task in Russian, got %q", title)
	}
}

// TestRemindersSwitchTogglesAnExistingWatcher: the task stays, its text is refreshed, and the switch
// is what changes.
func TestRemindersSwitchTogglesAnExistingWatcher(t *testing.T) {
	clock := &fakeClock{tasks: []map[string]any{
		{"id": "w1", "title": "Сторож календаря: напоминания за 30 минут", "enabled": true},
		{"id": "other", "title": "Почтовый секретарь", "enabled": true},
	}}
	srv := remindersServer(t, clock)

	res, err := http.Get(srv.URL + "/api/reminders")
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Enabled bool   `json:"enabled"`
		TaskID  string `json:"task_id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&state)
	res.Body.Close()
	if !state.Enabled || state.TaskID != "w1" {
		t.Fatalf("state = %+v", state)
	}

	res, err = http.Post(srv.URL+"/api/reminders", "application/json", strings.NewReader(`{"enabled":false}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(clock.toggled) != 1 || clock.toggled[0] != false {
		t.Fatalf("toggled = %v", clock.toggled)
	}
	// Turning it off must not rewrite the text: a paused task is still the task.
	if len(clock.updated) != 0 {
		t.Errorf("the text was rewritten while pausing: %v", clock.updated)
	}

	res, err = http.Post(srv.URL+"/api/reminders", "application/json", strings.NewReader(`{"enabled":true}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(clock.updated) != 1 || !strings.Contains(clock.updated[0], "notify_send") {
		t.Errorf("the text was not refreshed on resume: %v", clock.updated)
	}
}

// TestRemindersSpeakTheWindowsLanguage: an English window gets an English task name and text, and
// the task is still found afterwards — otherwise every language switch would leave a second watcher.
func TestRemindersSpeakTheWindowsLanguage(t *testing.T) {
	clock := &fakeClock{}
	srv := remindersServer(t, clock)

	res, err := http.Post(srv.URL+"/api/reminders", "application/json", strings.NewReader(`{"enabled":true,"lang":"en"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	title, _ := clock.created[0]["title"].(string)
	if !strings.HasPrefix(title, "Calendar watcher") {
		t.Fatalf("title = %q", title)
	}
	action, _ := clock.created[0]["action"].(string)
	for _, want := range []string{"You are the calendar watcher", "notify_send", "In N min:"} {
		if !strings.Contains(action, want) {
			t.Errorf("the English text does not mention %q", want)
		}
	}

	// The English task is found by the module: no second watcher is made on the next switch.
	clock.tasks = []map[string]any{{"id": "w-en", "title": title, "enabled": true}}
	buf := new(strings.Builder)
	if err := json.NewEncoder(buf).Encode(clock); err != nil {
		t.Fatal(err)
	}
	res, err = http.Post(srv.URL+"/api/reminders", "application/json", strings.NewReader(`{"enabled":false,"lang":"en"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(clock.created) != 1 {
		t.Errorf("a second watcher was created: %v", clock.created)
	}
	if len(clock.toggled) != 1 || clock.toggled[0] != false {
		t.Errorf("toggled = %v", clock.toggled)
	}
}
