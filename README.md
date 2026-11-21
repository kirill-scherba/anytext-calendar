# anytext-calendar — the calendar module of AnyText

A screen for a person over the calendar that `mail-mcp` already owns: a month, a week, a list, and an
editor for events, including the several calendars of one Google account with their colours.

The module is a separate process with its own HTTP server on loopback. AnyText starts it, proxies
`/api/module/calendar/...` to it, says who is asking, and tells it where the MCP gateway answers. The
calendar data comes from `mail-mcp`'s calendar tools through that gateway — the module holds no Google
token and no OAuth code of its own (docs/MODULES.md in `anytext-gui`).

```
                 ┌───────────────┐   /api/module/calendar/…   ┌──────────────────┐
person ─────────►│    AnyText    │───────────────────────────►│ anytext-calendar │
                 │  (proxy)      │  X-AnyText-Person, Rights  │  127.0.0.1:7840  │
                 └───────┬───────┘  ANYTEXT_MCP_PROXY        └────────┬─────────┘
                         │ gateway                                     │ JSON-RPC
                         ▼                                             ▼
                 ┌───────────────┐                            ┌──────────────────┐
                 │  MCP gateway  │◄───────────────────────────│    mail-mcp      │
                 └───────────────┘                            │ calendar_* tools │
                                                              └──────────────────┘
```

## What it needs

- AnyText 0.4.14 or newer: modules are started with `ANYTEXT_MCP_PROXY`.
- `mail-mcp` 1.1.1 or newer: the calendar tools answer with `structuredContent`, expand recurring
  events, list the calendars (`calendar_calendars`) and take all-day events as a date pair.
- An MCP gateway that passes `structuredContent` through (mcp-gateway as of 21.11.2026).

Without the last two the screen says so in one sentence instead of showing an empty month: "the server
answers without structuredContent: update mail-mcp to 1.1.1".

## Build and install

```bash
CGO_ENABLED=0 go build -o ~/.local/bin/anytext-calendar .

curl -s -X POST http://127.0.0.1:7787/api/command -H 'Content-Type: application/json' -d '{
  "command":"modules.install",
  "args":{"id":"calendar","name":"Календарь","names":{"ru":"Календарь","en":"Calendar"},"addr":"127.0.0.1:7840",
          "command":["'"$HOME"'/.local/bin/anytext-calendar","-addr","127.0.0.1:7840"],
          "auto_start":true}}'

curl -s http://127.0.0.1:7787/api/module/calendar/health   # {"status":"ok"}
```

The window shows the module under the puzzle button. Its own screens are at
`/api/module/calendar/ui/`.

## The screen

| Part | What it does |
|---|---|
| month | six weeks of the month, three events per day, `+N` for the rest |
| week | hours from 00 to 23, all-day events in a strip above, overlaps side by side |
| list | everything in the month, grouped by day |
| calendars | a chip per calendar with a checkbox and Google's own colour; the choice is remembered per browser |
| reminders | one switch: the calendar's own task in the installation's clock, which tells its owner 30 minutes before an event. The task is visible and editable in the tasks module; the delivery (chat, phone) belongs to the installation, not to this module |
| editor | title, calendar, all-day switch, start and end, place, description, guests; delete |

The screen is **one self-contained document**: the styles and the script are inside the page, not
separate files. A module's frame is filled through the relay, and a `<link>` or a `<script src>` from
that frame carries no account token — the relay refuses it, and a module split into files shows bare
HTML on a white background. This module learned it the hard way on 21.11.2026.

A click on an empty place of a day starts an event there; a click on an event opens it.

Editing one occurrence of a recurring event changes that occurrence only — the calendar API's own rule,
said on the screen. Moving an event to another calendar is not in this version: `calendar_update` has
no such operation.

## API

The screen and anything else may use it; the installation's proxy is the only way in.

| Request | Answer |
|---|---|
| `GET /health` | `{"status":"ok"}` |
| `GET /api/status` | module version, gateway, whether the calendar answers, and why not |
| `GET /api/calendars[?fresh=1]` | the calendars with colours, access roles and selection |
| `GET /api/events?from=…&to=…&calendars=a,b` | events of several calendars in one range, each with its calendar's name and colour; a broken calendar is named in `errors` and does not blank the screen |
| `POST /api/events` | create; all-day events take a date pair, timed ones RFC 3339 with an offset |
| `PATCH /api/events?id=…&calendar=…` | change only the fields sent; an empty string clears a field |
| `DELETE /api/events?id=…&calendar=…` | delete one event, or one occurrence |
| `GET /api/reminders` | whether the calendar watcher runs, its lead and its task id |
| `POST /api/reminders` | `{"enabled":true|false}` — create the watcher, rewrite its text, pause or resume it |

`from` and `to` are RFC 3339 with an offset: the browser writes its own.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `127.0.0.1:7840` | where the module listens (loopback only) |
| `-mcp` | `$ANYTEXT_MCP_PROXY` | the MCP gateway, `http://127.0.0.1:7711` when nothing is set |
| `-server` | `mail-mcp` | the MCP server that holds the calendar tools |

## Tests

```bash
go vet ./... && go test ./...
```

The tests use a fake gateway, so they need no token and no network: they check the reading of
`structuredContent`, the refusal of an older server, the shapes of start and end, the merge of several
calendars with one broken, the resolution of Google's `primary` alias, and the page's base address.
