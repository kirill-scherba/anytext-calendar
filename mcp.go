// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// mcp.go — the module's hand into the calendar that mail-mcp already owns.
//
// The module does not talk to Google itself: the tokens, the OAuth client and the refresh logic live
// in mail-mcp, and a second copy of them would be a second thing to fix when Google changes a scope.
// The installation's MCP gateway answers an ordinary JSON-RPC request at <base>/<server>, and the
// calendar tools answer with structuredContent - the typed answer this module draws (21.11.2026).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// calendarClient calls the calendar tools of one MCP server through the gateway.
type calendarClient struct {
	base   string // the gateway, e.g. http://127.0.0.1:7711
	server string // the MCP server that holds the tools, e.g. mail-mcp
	http   *http.Client
	id     atomic.Int64

	mu        sync.Mutex
	calendars []Calendar
	cachedAt  time.Time
}

// calendarCacheTTL is how long the calendar list is trusted. The list changes when a person adds a
// calendar, not between two screen refreshes.
const calendarCacheTTL = 5 * time.Minute

func newCalendarClient(base, server string) *calendarClient {
	return &calendarClient{
		base:   base,
		server: server,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

// Calendar is one calendar of the person's list, as mail-mcp describes it.
type Calendar struct {
	ID              string `json:"id"`
	Summary         string `json:"summary"`
	Description     string `json:"description,omitempty"`
	Primary         bool   `json:"primary"`
	AccessRole      string `json:"access_role,omitempty"`
	BackgroundColor string `json:"background_color,omitempty"`
	ForegroundColor string `json:"foreground_color,omitempty"`
	Selected        bool   `json:"selected"`
	TimeZone        string `json:"time_zone,omitempty"`
}

// Attendee is one person on an event.
type Attendee struct {
	Email          string `json:"email"`
	DisplayName    string `json:"display_name,omitempty"`
	ResponseStatus string `json:"response_status,omitempty"`
}

// Event is one event of a calendar. Start and End are RFC 3339 timestamps for a timed event and plain
// dates (2026-11-21) for an all-day one; AllDay says which.
type Event struct {
	ID               string     `json:"id"`
	CalendarID       string     `json:"calendar_id,omitempty"`
	Summary          string     `json:"summary"`
	Description      string     `json:"description,omitempty"`
	Location         string     `json:"location,omitempty"`
	Status           string     `json:"status,omitempty"`
	HTMLLink         string     `json:"html_link,omitempty"`
	Start            string     `json:"start"`
	End              string     `json:"end"`
	AllDay           bool       `json:"all_day"`
	TimeZone         string     `json:"time_zone,omitempty"`
	RecurringEventID string     `json:"recurring_event_id,omitempty"`
	Recurrence       []string   `json:"recurrence,omitempty"`
	Attendees        []Attendee `json:"attendees,omitempty"`
}

// shownEvent is an event with what a screen needs to paint it: the name and the colour of the
// calendar it belongs to.
type shownEvent struct {
	Event
	CalendarName  string `json:"calendar_name,omitempty"`
	CalendarColor string `json:"calendar_color,omitempty"`
}

// rpcResponse is the part of a JSON-RPC answer this module reads.
type rpcResponse struct {
	Result *struct {
		Content           []struct{ Type, Text string } `json:"content"`
		StructuredContent json.RawMessage               `json:"structuredContent"`
		IsError           bool                          `json:"isError"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// call answers with the structuredContent of one tool: the typed answer, not the sentence the agent
// reads. An older mail-mcp has no structured answer, and saying so plainly is better than parsing
// Russian text out of a sentence.
func (c *calendarClient) call(ctx context.Context, tool string, args any) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      c.id.Add(1),
		"method":  "tools/call",
		"params":  map[string]any{"name": tool, "arguments": args},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/"+url.PathEscape(c.server), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("MCP-шлюз %s не ответил: %w", c.base, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MCP-сервер %s: %s: %s", c.server, resp.Status, trimText(data))
	}

	var out rpcResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("ответ %s не разобран: %w", c.server, err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("%s: %s", c.server, out.Error.Message)
	}
	if out.Result == nil {
		return nil, fmt.Errorf("%s ответил без результата", c.server)
	}
	if out.Result.IsError {
		return nil, fmt.Errorf("%s: %s", c.server, firstText(out.Result.Content))
	}
	if len(out.Result.StructuredContent) == 0 {
		return nil, fmt.Errorf("сервер %s отвечает без structuredContent: обновите mail-mcp до 1.1.1", c.server)
	}
	return out.Result.StructuredContent, nil
}

// Calendars returns the person's calendar list, cached for a few minutes.
func (c *calendarClient) Calendars(ctx context.Context, fresh bool) ([]Calendar, error) {
	c.mu.Lock()
	if !fresh && len(c.calendars) > 0 && time.Since(c.cachedAt) < calendarCacheTTL {
		out := c.calendars
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()

	raw, err := c.call(ctx, "calendar_calendars", map[string]any{})
	if err != nil {
		return nil, err
	}
	var answer struct {
		Calendars []Calendar `json:"calendars"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return nil, fmt.Errorf("список календарей не разобран: %w", err)
	}

	c.mu.Lock()
	c.calendars, c.cachedAt = answer.Calendars, time.Now()
	c.mu.Unlock()
	return answer.Calendars, nil
}

// CalendarByID finds one calendar in the list.
func (c *calendarClient) CalendarByID(ctx context.Context, id string) (Calendar, bool) {
	list, err := c.Calendars(ctx, false)
	if err != nil {
		return Calendar{}, false
	}
	for _, cal := range list {
		if cal.ID == id {
			return cal, true
		}
	}
	return Calendar{}, false
}

// Events reads one calendar for a range. from and to are RFC 3339 timestamps with offset.
func (c *calendarClient) Events(ctx context.Context, calendarID string, from, to time.Time) ([]Event, error) {
	raw, err := c.call(ctx, "calendar_list", map[string]any{
		"calendar_id": calendarID,
		"time_min":    from.Format(time.RFC3339),
		"time_max":    to.Format(time.RFC3339),
		// A month of a busy calendar does not fit into one page of 250; 2500 is what mail-mcp reads.
		"max_results": 2500,
	})
	if err != nil {
		return nil, err
	}
	var answer struct {
		Events []Event `json:"events"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return nil, fmt.Errorf("события не разобраны: %w", err)
	}
	out := make([]Event, 0, len(answer.Events))
	for _, e := range answer.Events {
		if e.CalendarID == "" {
			e.CalendarID = calendarID
		}
		out = append(out, e)
	}
	return out, nil
}

// Create makes an event and answers with what the calendar now holds.
func (c *calendarClient) Create(ctx context.Context, args map[string]any) (Event, error) {
	raw, err := c.call(ctx, "calendar_create", args)
	if err != nil {
		return Event{}, err
	}
	return eventFrom(raw)
}

// Update changes the fields a person edited and answers with the event as it is now.
func (c *calendarClient) Update(ctx context.Context, args map[string]any) (Event, error) {
	raw, err := c.call(ctx, "calendar_update", args)
	if err != nil {
		return Event{}, err
	}
	return eventFrom(raw)
}

// Delete removes one event (one occurrence, for a recurring series).
func (c *calendarClient) Delete(ctx context.Context, calendarID, eventID string) error {
	_, err := c.call(ctx, "calendar_delete", map[string]any{"calendar_id": calendarID, "event_id": eventID})
	return err
}

// eventFrom reads the event out of the answer of calendar_create and calendar_update.
func eventFrom(raw json.RawMessage) (Event, error) {
	var answer struct {
		Event Event `json:"event"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return Event{}, fmt.Errorf("событие не разобрано: %w", err)
	}
	if answer.Event.ID == "" {
		return Event{}, fmt.Errorf("сервер не вернул событие")
	}
	return answer.Event, nil
}

// SortEvents orders a day the way a person reads it: all-day events first, then by start.
func SortEvents(events []shownEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.AllDay != b.AllDay {
			return a.AllDay
		}
		at, aok := eventStart(a.Event)
		bt, bok := eventStart(b.Event)
		if !aok || !bok {
			return aok
		}
		return at.Before(bt)
	})
}

// eventStart reads the start of an event as a time: the date of an all-day one at local midnight.
func eventStart(e Event) (time.Time, bool) {
	if e.AllDay {
		t, err := time.ParseInLocation("2006-01-02", e.Start, time.Local)
		return t, err == nil
	}
	t, err := time.Parse(time.RFC3339, e.Start)
	return t, err == nil
}

func firstText(items []struct{ Type, Text string }) string {
	if len(items) == 0 {
		return "неизвестная ошибка"
	}
	return items[0].Text
}

func trimText(data []byte) string {
	const max = 300
	if len(data) > max {
		return string(data[:max]) + "…"
	}
	return string(data)
}
