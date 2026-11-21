// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// watcher.go — what the calendar's own task does. One text per language, used when the task is
// created and rewritten when the switch is used: the calendars and the lead live here, the delivery
// does not (the installation owns the channels).
package main

import "fmt"

// watchLead is how long before an event the person is told.
const watchLead = 30

// watchEvery is how often the task looks at the calendar.
const watchEvery = "15m"

// watchCalendars are the calendars a reminder is worth: the person's own and the family one. The
// week numbers and the holidays calendars are noise, and an event there is not a meeting.
var watchCalendars = []string{
	"kiriushka@gmail.com",
	"family16674268680589041019@group.calendar.google.com",
}

// watcherTitles are the names the task may carry, one per language the module speaks. The module
// looks for either: a person who switched the window from Russian to English on Tuesday must not end
// up with two watchers on Wednesday.
var watcherTitles = []string{"Сторож календаря", "Calendar watcher"}

// watcherTitle is the name in one language.
func watcherTitle(lang string) string {
	if lang == "en" {
		return fmt.Sprintf("Calendar watcher: reminders %d minutes ahead", watchLead)
	}
	return fmt.Sprintf("Сторож календаря: напоминания за %d минут", watchLead)
}

// watchAction is the task text in the language of the window. It says what to do and leaves the
// "where" to the installation: the notify_send tool delivers to the chat and the phone, and the
// module holds no channel of its own.
func watchAction(lang string) string {
	calendars := ""
	for _, id := range watchCalendars {
		calendars += "\n   - " + id
	}
	if lang == "en" {
		return fmt.Sprintf(`You are the calendar watcher of the owner. Work quietly and briefly, no reports.

1. Read the events: call calendar_list (server mail-mcp) for the calendars:%s
   Range: time_min = now, time_max = now + 2 hours, max_results = 50.
   Pass the time as RFC 3339 with the +03:00 offset (Moscow).

2. Keep only the events that start within the next %d minutes and have not started yet.
   Skip all-day events.

3. Keep the state in /home/kirill/.config/teonet/anytext/scheduler/state/calendar-watch.json
   Format: {"sent": {"<id>|<start>": "<when sent>"}}
   No file — create it. The key is the event id and its start. Drop entries older than two days.

4. For every event without an entry call notify_send(source="calendar",
   text="In N min: <title> — <HH:MM>, <place>") — exactly once per event.
   After sending, write the key into the file.

5. No new events — finish silently: do not call notify_send, write nothing.

6. Between 23:00 and 08:00 Moscow time send nothing.

7. Use no other tools: only calendar_list, notify_send and that file.`, calendars, watchLead)
	}
	return fmt.Sprintf(`Ты — сторож календаря Кирилла. Работай тихо и коротко, без отчётов.

1. Получи события: вызови calendar_list (сервер mail-mcp) для календарей:%s
   Диапазон: time_min = сейчас, time_max = сейчас + 2 часа, max_results = 50.
   Время передавай в RFC 3339 со смещением +03:00 (Москва).

2. Оставь только события, которые начнутся в ближайшие %d минут и ещё не начались.
   События «на весь день» (all_day) пропускай.

3. Состояние веди в файле /home/kirill/.config/teonet/anytext/scheduler/state/calendar-watch.json
   Формат: {"sent": {"<id>|<start>": "<когда отправлено>"}}
   Нет файла — создай. Ключ — id события и время начала. Записи старше двух дней удаляй.

4. Для каждого события без записи вызови notify_send(source="calendar",
   text="Через N мин: <название> — <ЧЧ:ММ>, <место>") — ровно один раз на событие.
   После отправки впиши ключ в файл.

5. Новых событий нет — завершись молча: notify_send не вызывай, ничего не пиши.

6. С 23:00 до 08:00 по Москве ничего не отправляй.

7. Другими инструментами не пользуйся: только calendar_list, notify_send и этот файл.`,
		calendars, watchLead)
}
