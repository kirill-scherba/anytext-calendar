// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// watcher.go — what the calendar's own task does. One text, used when the task is created and
// rewritten when it already exists: the calendars and the lead live here, the delivery does not.
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

// watchAction is the task text. It says what to do and leaves the "where" to the installation: the
// notify_send tool delivers to the chat and the phone, and the module holds no channel of its own.
func watchAction() string {
	calendars := ""
	for _, id := range watchCalendars {
		calendars += "\n   - " + id
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
