# Context — anytext-calendar

## Current state

The module shows Google Calendar of the person: a month, a week, a list, and an editor for events,
over several calendars of one account. It is an AnyText module (a separate process with its own HTTP
server, docs/MODULES.md in anytext-gui, version 0.4.14 and newer) and takes its data from the calendar
tools of `mail-mcp` (1.1.1 and newer) through the installation's MCP gateway.

## Decisions

- 21.11.2026: the module does **not** talk to Google itself. The OAuth client, the token store and the
  refresh logic live in `mail-mcp`; a second copy would be a second thing to fix when Google changes a
  scope. The module calls `calendar_calendars`, `calendar_list`, `calendar_create`, `calendar_update`
  and `calendar_delete` at `<ANYTEXT_MCP_PROXY>/mail-mcp`.
- 21.11.2026: the module reads `structuredContent` and nothing else. Parsing the Russian sentence of a
  text answer would break on a wording change; an older server is reported as "update mail-mcp to
  1.1.1" instead of being half-read.
- 21.11.2026: the screens are plain HTML, CSS and JavaScript embedded in the binary. No framework and
  no build step: a module is replaced by replacing one file.
- 21.11.2026: the theme and the language come from the shell's own keys (`localStorage`
  `anytext-theme`, `anytext.lang`): the module is served from the same origin, and a frame with a look
  of its own is a frame nobody asked for.
- 21.11.2026: no MCP tools of its own. The agent already has the calendar through `mail-mcp`, and a
  second set of tools over the same data would be a second way to describe the same thing.
- 21.11.2026: `primary` is resolved to the real calendar id before an event reaches the screen:
  Google calls the main calendar `primary` in an API call and by its address in the calendar list.
- 21.11.2026: one broken calendar does not blank the screen. Its error is named in `errors`; the events
  of the calendars that answered are shown.

## Limits of this version

- Moving an event to another calendar is not possible: `calendar_update` has no such operation.
- Editing a recurring event changes one occurrence (the calendar API's rule), and the editor says so.
  A series itself (its recurrence rule) is not edited here.
- Guests are added and removed by address; Google's notifications are off in `mail-mcp` by design
  (`sendUpdates=none`).
- The week view lays overlapping events side by side in equal columns; it does not pack them.
- The screen asks for up to 2500 events per calendar per range.

## Lessons

- 21.11.2026: **a module's screen must be one document.** The styles and the script went as separate
  files (`ui/style.css`, `ui/app.js`) and everything worked at the machine. In the remote client the
  person saw only the bare toolbar on a white background: the shell fetches the page with the account
  token and puts it into the frame, but a `<link>` or a `<script src>` inside that frame is a request
  of the browser, which carries no token - the relay refuses it. The same reason keeps every module
  that works remotely single-file. The page now carries `<style>` and `<script>` inline, built from
  the same files at start, and `TestPageIsSelfContained` refuses a page that asks for a separate file.
  Verified without the relay: a local proxy that answers 401 to every `/api/*` without a token, and a
  harness page that repeats the shell's work (fetch with the token, inject `<base>` and a `fetch`
  wrapper, put the result into `srcdoc`). The calendar renders fully there while the old page would
  have been refused at its stylesheet.

## Recent changes

- 21.11.2026: 0.1.1 — the screen is one self-contained document, so the remote client shows the same
  calendar as the local one.
- 21.11.2026: first version 0.1.0 — month, week, list, editor, several calendars, the calendar chips.
  Checked against the live calendar of the owner's account through a gateway and directly through
  `mail-mcp`'s stdio (`calendar_calendars`, expanded recurring events, all-day events by a date pair,
  clearing a description).

## 21.11.2026 — напоминания из календаря: галка в модуле, доставка у установки

- Кирилл: «в Telegram бот ещё», и «галку включения/выключения сделать в модуле календаря, а настройку
  куда отправлять — потом, в настройках AnyText, чтобы все оповещения шли через одно место».
- В установке появилась **одна дверь уведомлений**: MCP-сервер `notify` с инструментом `notify_send`
  (`/api/mcp/notify`), команда `notify.send` и `Server.NotifyOwner` — чат (плагин opencode на 7799) и
  телефон (Telegram через сайт → API → бот). Каналы независимы: один упавший назван, другой работает.
  Проверено: `{"chat":"ok","telegram":"ok"}`.
- В календаре — кнопка «Напоминания»: `GET/POST /api/reminders`. Модуль находит задачу сторожа по
  префиксу названия, создаёт её при включении, переписывает текст при каждом включении и ставит на
  паузу при выключении; сама задача живёт в планировщике и видна в модуле «Задачи».
- Сторож больше не зовёт `notify_opencode`: его текст говорит `notify_send`, поэтому напоминание
  приходит и в чат, и в телефон, а куда именно — решает установка (раздел настроек — следующий шаг).
