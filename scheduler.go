// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// scheduler.go — the module's hand into the installation's clock, for one thing only: the calendar
// watcher. The task itself is an ordinary scheduled task, visible and editable in the tasks module;
// the calendar keeps it alive or paused and builds its text (21.11.2026).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// watcherTitle names the task the calendar owns. A person may rename it in the tasks module; the
// search is by prefix so a renamed tail still matches.
const watcherTitle = "Сторож календаря"

// schedulerClient calls the tools of one installation's schedule.
type schedulerClient struct {
	base string
	http *http.Client
}

func newSchedulerClient(base string) *schedulerClient {
	return &schedulerClient{base: base, http: &http.Client{Timeout: 60 * time.Second}}
}

// scheduled is the part of a task this module reads.
type scheduled struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Enabled bool   `json:"enabled"`
}

// call runs one tool of /api/mcp/scheduler and answers with its typed value.
func (c *schedulerClient) call(ctx context.Context, tool string, args map[string]any) (json.RawMessage, error) {
	if args == nil {
		args = map[string]any{}
	}
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/mcp/scheduler", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("установка %s не ответила: %w", c.base, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var answer struct {
		Result struct {
			StructuredContent json.RawMessage `json:"structuredContent"`
			Content           []struct{ Text string }
			IsError           bool `json:"isError"`
		} `json:"result"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.Unmarshal(data, &answer); err != nil {
		return nil, fmt.Errorf("ответ установки не разобран: %w", err)
	}
	if answer.Error != nil {
		return nil, fmt.Errorf("%s", answer.Error.Message)
	}
	if answer.Result.IsError {
		text := ""
		if len(answer.Result.Content) > 0 {
			text = answer.Result.Content[0].Text
		}
		return nil, fmt.Errorf("%s", text)
	}
	if len(answer.Result.StructuredContent) > 0 && string(answer.Result.StructuredContent) != "null" {
		return answer.Result.StructuredContent, nil
	}
	if len(answer.Result.Content) > 0 {
		return json.RawMessage(answer.Result.Content[0].Text), nil
	}
	return nil, fmt.Errorf("пустой ответ инструмента %s", tool)
}

// Watcher finds the calendar's own task, or answers false.
func (c *schedulerClient) Watcher(ctx context.Context) (scheduled, bool, error) {
	raw, err := c.call(ctx, "scheduler_list", nil)
	if err != nil {
		return scheduled{}, false, err
	}
	var answer struct {
		Tasks []scheduled `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return scheduled{}, false, fmt.Errorf("список задач не разобран: %w", err)
	}
	for _, task := range answer.Tasks {
		if strings.HasPrefix(task.Title, watcherTitle) {
			return task, true, nil
		}
	}
	return scheduled{}, false, nil
}

// Create writes the watcher task and answers with it.
func (c *schedulerClient) Create(ctx context.Context, action string, when time.Time, every string) (scheduled, error) {
	raw, err := c.call(ctx, "scheduler_create", map[string]any{
		"title":   watcherTitle + ": напоминания за 30 минут",
		"action":  action,
		"when":    when.Format(time.RFC3339),
		"every":   every,
		"enabled": true,
	})
	if err != nil {
		return scheduled{}, err
	}
	var task scheduled
	if err := json.Unmarshal(raw, &task); err != nil {
		return scheduled{}, fmt.Errorf("задача не разобрана: %w", err)
	}
	return task, nil
}

// SetAction rewrites the text of the watcher: the calendars and the lead may change, the clock stays.
func (c *schedulerClient) SetAction(ctx context.Context, id, action string) error {
	_, err := c.call(ctx, "scheduler_update", map[string]any{"id": id, "action": action})
	return err
}

// Toggle pauses or resumes the watcher.
func (c *schedulerClient) Toggle(ctx context.Context, id string, enabled bool) error {
	_, err := c.call(ctx, "scheduler_toggle", map[string]any{"id": id, "enabled": enabled})
	return err
}
