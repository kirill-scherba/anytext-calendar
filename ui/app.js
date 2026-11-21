// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// app.js — the screen of the calendar module. No framework and no build step: the module is a binary
// with its pages inside it, and a page that needs a toolchain to change is a page nobody changes.
//
// The data comes from the module's own API (./api/...): the module asks mail-mcp through the
// installation's MCP gateway. The screen never talks to Google.

'use strict'

// --- language and theme ------------------------------------------------------------------------
// Both come from the shell's own keys: the module is served from the same origin, and a person who
// read the interface in Russian yesterday reads it in Russian today (docs/MODULES.md).

const LANGS = ['ru', 'en']

function storedLang() {
  try {
    const value = localStorage.getItem('anytext.lang')
    if (LANGS.includes(value)) return value
  } catch (e) { /* private mode: the system language is enough */ }
  const system = (navigator.language || '').toLowerCase()
  return system.startsWith('ru') ? 'ru' : 'en'
}

let lang = storedLang()

const STRINGS = {
  ru: {
    today: 'Сегодня', month: 'Месяц', week: 'Неделя', agenda: 'Список',
    newEvent: '+ Событие', event: 'Событие', newEventTitle: 'Новое событие',
    title: 'Название', calendar: 'Календарь', allDay: 'Весь день',
    start: 'Начало', end: 'Конец', location: 'Место', description: 'Описание',
    attendees: 'Участники (адреса через запятую)',
    repeatHint: 'Это повторяющееся событие: правка изменит только этот раз.',
    inGoogle: 'Открыть в Google Календаре',
    watch: 'Напоминания',
    watchTitle: 'Напоминать за 30 минут о событиях основного и семейного календаря',
    watchOn: 'Напоминания включены',
    watchOff: 'Напоминания выключены',
    watchFailed: 'Не удалось переключить напоминания',
    delete: 'Удалить', cancel: 'Отмена', save: 'Сохранить',
    needTitle: 'Нужно название события',
    endBeforeStart: 'Конец должен быть позже начала',
    badTimes: 'Проверьте дату и время',
    allDayLabel: 'весь день', more: 'ещё',
    loading: 'Загрузка…', empty: 'Ничего не запланировано',
    deleteQuestion: 'Удалить событие?',
    readOnly: 'только чтение',
    gatewayOff: 'Календарь недоступен',
    noEvents: 'На этот промежуток ничего не запланировано',
    weekdays: ['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс'],
  },
  en: {
    today: 'Today', month: 'Month', week: 'Week', agenda: 'List',
    newEvent: '+ Event', event: 'Event', newEventTitle: 'New event',
    title: 'Title', calendar: 'Calendar', allDay: 'All day',
    start: 'Start', end: 'End', location: 'Location', description: 'Description',
    attendees: 'Guests (addresses, comma separated)',
    repeatHint: 'This is a recurring event: the change applies to this occurrence only.',
    inGoogle: 'Open in Google Calendar',
    watch: 'Reminders',
    watchTitle: 'Remind me 30 minutes before events of the main and family calendars',
    watchOn: 'Reminders are on',
    watchOff: 'Reminders are off',
    watchFailed: 'The reminders could not be switched',
    delete: 'Delete', cancel: 'Cancel', save: 'Save',
    needTitle: 'An event needs a title',
    endBeforeStart: 'The end must be after the start',
    badTimes: 'Check the date and the time',
    allDayLabel: 'all day', more: 'more',
    loading: 'Loading…', empty: 'Nothing planned',
    deleteQuestion: 'Delete this event?',
    readOnly: 'read only',
    gatewayOff: 'The calendar is not reachable',
    noEvents: 'Nothing planned for this period',
    weekdays: ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'],
  },
}

/** t is a word of the screen in the person's language. */
function t(key) {
  const table = STRINGS[lang] || STRINGS.en
  return table[key] !== undefined ? table[key] : key
}

function locale() {
  return lang === 'ru' ? 'ru-RU' : 'en-GB'
}

/** applyTheme copies the shell's theme into the page: the frame must not be a black hole in a light
 *  window (or the other way round). */
function applyTheme() {
  let theme = 'dark'
  try {
    theme = localStorage.getItem('anytext-theme') || 'dark'
  } catch (e) { /* private mode */ }
  document.documentElement.classList.toggle('dark', theme !== 'light')
}

// --- small helpers ------------------------------------------------------------------------------

const $ = (id) => document.getElementById(id)

/** esc makes text safe for innerHTML: an event called "<b>" is a title, not markup. */
function esc(value) {
  return String(value === undefined || value === null ? '' : value)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;')
}

const pad = (n) => String(n).padStart(2, '0')

/** dateKey is the local day of a Date as YYYY-MM-DD. */
function dateKey(d) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** parseDateKey reads YYYY-MM-DD as local midnight. */
function parseDateKey(key) {
  const [y, m, d] = key.split('-').map(Number)
  return new Date(y, m - 1, d)
}

function startOfDay(d) {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate())
}

function addDays(d, n) {
  const out = new Date(d)
  out.setDate(out.getDate() + n)
  return out
}

/** startOfWeek is Monday: the week of the calendar of the country this product is made for. */
function startOfWeek(d) {
  const day = startOfDay(d)
  const shift = (day.getDay() + 6) % 7
  return addDays(day, -shift)
}

/** rfc3339 writes a Date with its local offset: what mail-mcp demands of a timestamp. */
function rfc3339(d) {
  const offset = -d.getTimezoneOffset()
  const sign = offset >= 0 ? '+' : '-'
  const abs = Math.abs(offset)
  return `${dateKey(d)}T${pad(d.getHours())}:${pad(d.getMinutes())}:00${sign}${pad(Math.floor(abs / 60))}:${pad(abs % 60)}`
}

function rfc3339FromParts(date, time) {
  const [y, m, d] = date.split('-').map(Number)
  const [hh, mm] = (time || '00:00').split(':').map(Number)
  return rfc3339(new Date(y, m - 1, d, hh || 0, mm || 0))
}

// --- the state ----------------------------------------------------------------------------------

const state = {
  view: 'month',
  cursor: startOfDay(new Date()),
  calendars: [],
  selected: new Set(),
  events: [],
  errors: [],
  ready: false,
  reminders: { enabled: false, ready: false },
  statusError: '',
  version: '',
  gateway: '',
  loading: false,
}

const SELECTED_KEY = 'anytextCalendar.selected'
const VIEW_KEY = 'anytextCalendar.view'

function loadSelection() {
  try {
    const raw = localStorage.getItem(SELECTED_KEY)
    if (!raw) return null
    const list = JSON.parse(raw)
    return Array.isArray(list) ? new Set(list) : null
  } catch (e) {
    return null
  }
}

function saveSelection() {
  try {
    localStorage.setItem(SELECTED_KEY, JSON.stringify([...state.selected]))
  } catch (e) { /* private mode: the choice lives in this tab */ }
}

/** ensureSelection fills the chosen calendars on the first run: what Google itself shows, and the
 *  primary calendar always. A person who unchecks everything keeps his empty screen on purpose. */
function ensureSelection() {
  const stored = loadSelection()
  if (stored) {
    state.selected = stored
    return
  }
  state.selected = new Set()
  for (const cal of state.calendars) {
    if (cal.selected || cal.primary) state.selected.add(cal.id)
  }
  if (state.selected.size === 0 && state.calendars.length) state.selected.add(state.calendars[0].id)
  saveSelection()
}

// --- the API ------------------------------------------------------------------------------------

class ApiError extends Error {}

async function api(path, options) {
  const res = await fetch(path, options)
  let data = null
  try {
    data = await res.json()
  } catch (e) {
    throw new ApiError(`${res.status} ${res.statusText}`)
  }
  if (!res.ok) throw new ApiError((data && data.error) || `${res.status} ${res.statusText}`)
  return data
}

async function loadStatus() {
  try {
    const st = await api('api/status')
    state.ready = !!st.ready
    state.statusError = st.error || ''
    state.version = st.version || ''
    state.gateway = st.gateway || ''
    $('footInfo').textContent = `v${state.version} · ${st.server || 'mail-mcp'} · ${state.gateway}`
    if (!st.ready) showNotice(`${t('gatewayOff')}: ${state.statusError || st.server || ''}`)
  } catch (e) {
    state.ready = false
    showNotice(`${t('gatewayOff')}: ${e.message}`)
  }
}

async function loadCalendars(fresh) {
  const answer = await api('api/calendars' + (fresh ? '?fresh=1' : ''))
  state.calendars = answer.calendars || []
  ensureSelection()
  renderCalendars()
}

async function loadReminders() {
  try {
    const r = await api('api/reminders')
    state.reminders = { enabled: !!r.enabled, ready: true }
    if (r.error) showNotice(`${t('watchFailed')}: ${r.error}`, true)
  } catch (e) {
    state.reminders = { enabled: false, ready: false }
  }
  renderWatch()
}

/** renderWatch paints the switch: a person must see whether reminders run without opening anything. */
function renderWatch() {
  const button = $('watch')
  if (!button) return
  const on = state.reminders.enabled
  button.classList.toggle('active', on)
  button.title = t('watchTitle')
  button.setAttribute('aria-pressed', on ? 'true' : 'false')
}

async function toggleReminders() {
  const button = $('watch')
  button.disabled = true
  try {
    const r = await api('api/reminders', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: !state.reminders.enabled, lang }),
    })
    state.reminders = { enabled: !!r.enabled, ready: true }
    hideNotice()
  } catch (e) {
    showNotice(`${t('watchFailed')}: ${e.message}`)
  } finally {
    button.disabled = false
    renderWatch()
  }
}

async function loadEvents() {
  const range = rangeOfView()
  const ids = [...state.selected]
  if (!ids.length) {
    state.events = []
    state.errors = []
    return
  }
  const query = `api/events?from=${encodeURIComponent(rfc3339(range.from))}` +
    `&to=${encodeURIComponent(rfc3339(range.to))}` +
    `&calendars=${encodeURIComponent(ids.join(','))}`
  const answer = await api(query)
  state.events = answer.events || []
  state.errors = answer.errors || []
}

/** rangeOfView is the half-open window of the current screen, in local time. */
function rangeOfView() {
  if (state.view === 'week') {
    const from = startOfWeek(state.cursor)
    return { from, to: addDays(from, 7) }
  }
  if (state.view === 'month') {
    const first = new Date(state.cursor.getFullYear(), state.cursor.getMonth(), 1)
    const from = startOfWeek(first)
    return { from, to: addDays(from, 42) }
  }
  const from = new Date(state.cursor.getFullYear(), state.cursor.getMonth(), 1)
  const to = new Date(state.cursor.getFullYear(), state.cursor.getMonth() + 1, 1)
  return { from, to }
}

// --- event timing -------------------------------------------------------------------------------

/** eventStartDay is the first local day of an event. */
function eventStartDay(ev) {
  if (ev.all_day) return parseDateKey(ev.start)
  return startOfDay(new Date(ev.start))
}

/** eventDayKeys lists every local day an event touches: a week-long all-day event must be visible
 *  on every one of its days, not only on the first. */
function eventDayKeys(ev) {
  const keys = []
  const first = eventStartDay(ev)
  let last
  if (ev.all_day) {
    // The end of an all-day event is the day after the last day in the calendar API.
    const end = parseDateKey(ev.end || ev.start)
    last = addDays(end, -1)
    if (last < first) last = first
  } else {
    last = startOfDay(new Date(ev.end))
    // An event that ends at midnight ends on the day before.
    const end = new Date(ev.end)
    if (end.getHours() === 0 && end.getMinutes() === 0 && last > first) last = addDays(last, -1)
  }
  for (let d = first; d <= last; d = addDays(d, 1)) keys.push(dateKey(d))
  return keys
}

function timeLabel(ev) {
  if (ev.all_day) return t('allDayLabel')
  const s = new Date(ev.start)
  const e = new Date(ev.end)
  const sameDay = dateKey(s) === dateKey(e)
  if (sameDay) return `${pad(s.getHours())}:${pad(s.getMinutes())}–${pad(e.getHours())}:${pad(e.getMinutes())}`
  return `${pad(s.getHours())}:${pad(s.getMinutes())} → ${pad(e.getDate())}.${pad(e.getMonth() + 1)}`
}

/** calendarColor finds the colour of the calendar an event belongs to. */
function calendarColor(id) {
  const cal = state.calendars.find((c) => c.id === id)
  return (cal && cal.background_color) || '#0d6efd'
}

/** chipStyle paints an event with its calendar's colour, softly enough to read the title. */
function chipStyle(id) {
  const color = calendarColor(id)
  return `border-left-color:${color};background:color-mix(in srgb, ${color} 18%, transparent)`
}

// --- rendering ----------------------------------------------------------------------------------

function render() {
  for (const button of document.querySelectorAll('[data-view]')) {
    button.classList.toggle('active', button.dataset.view === state.view)
  }
  $('period').textContent = periodLabel()
  const view = $('view')
  view.innerHTML = ''
  if (state.errors.length) {
    const names = state.errors.map((e) => e.summary || e.calendar_id).join(', ')
    showNotice(`${t('gatewayOff')} (${names}): ${state.errors[0].error}`)
  } else {
    hideNotice()
  }
  if (state.view === 'month') renderMonth(view)
  else if (state.view === 'week') renderWeek(view)
  else renderAgenda(view)
}

function periodLabel() {
  const d = state.cursor
  if (state.view === 'week') {
    const from = startOfWeek(d)
    const to = addDays(from, 6)
    const sameMonth = from.getMonth() === to.getMonth()
    const left = from.toLocaleDateString(locale(), { day: 'numeric', month: sameMonth ? undefined : 'short' })
    const right = to.toLocaleDateString(locale(), { day: 'numeric', month: 'short', year: 'numeric' })
    return `${left} – ${right}`
  }
  return d.toLocaleDateString(locale(), { month: 'long', year: 'numeric' })
}

function weekdayNames() {
  return (STRINGS[lang] || STRINGS.en).weekdays
}

function renderMonth(root) {
  const grid = document.createElement('div')
  grid.className = 'month'
  const names = weekdayNames()
  for (const name of names) {
    const head = document.createElement('div')
    head.className = 'weekday'
    head.textContent = name
    grid.appendChild(head)
  }

  const byDay = new Map()
  for (const ev of state.events) {
    for (const key of eventDayKeys(ev)) {
      if (!byDay.has(key)) byDay.set(key, [])
      byDay.get(key).push(ev)
    }
  }

  const from = startOfWeek(new Date(state.cursor.getFullYear(), state.cursor.getMonth(), 1))
  const todayKey = dateKey(new Date())
  for (let i = 0; i < 42; i++) {
    const day = addDays(from, i)
    const key = dateKey(day)
    const cell = document.createElement('div')
    cell.className = 'day'
    if (day.getMonth() !== state.cursor.getMonth()) cell.classList.add('out')
    if (key === todayKey) cell.classList.add('today')
    cell.dataset.date = key
    cell.innerHTML = `<div class="num">${day.getDate()}</div>`

    const list = (byDay.get(key) || []).slice().sort((a, b) => {
      if (a.all_day !== b.all_day) return a.all_day ? -1 : 1
      return new Date(a.start) - new Date(b.start)
    })
    const shown = list.slice(0, 3)
    for (const ev of shown) {
      const chip = document.createElement('button')
      chip.type = 'button'
      chip.className = 'chip'
      chip.dataset.id = ev.id
      chip.dataset.calendar = ev.calendar_id || ''
      chip.style.cssText = chipStyle(ev.calendar_id)
      chip.title = `${ev.summary}\n${timeLabel(ev)}`
      chip.innerHTML = `<span class="t">${ev.all_day ? '' : esc(timeLabel(ev).split('–')[0])}</span>${esc(ev.summary)}`
      cell.appendChild(chip)
    }
    if (list.length > shown.length) {
      const more = document.createElement('div')
      more.className = 'more'
      more.textContent = `+${list.length - shown.length} ${t('more')}`
      more.title = list.slice(shown.length).map((e) => e.summary).join('\n')
      cell.appendChild(more)
    }
    grid.appendChild(cell)
  }
  root.appendChild(grid)
}

function renderWeek(root) {
  const first = startOfWeek(state.cursor)
  const names = weekdayNames()
  const todayKey = dateKey(new Date())

  // Two grids with the same columns: the head stays on top while the hours scroll under it, so a
  // person at 15:00 still sees which day is which.
  const head = document.createElement('div')
  head.className = 'week week-head'
  head.appendChild(cell('div', 'corner'))
  for (let i = 0; i < 7; i++) {
    const day = addDays(first, i)
    const headCell = document.createElement('div')
    headCell.className = 'weekday' + (dateKey(day) === todayKey ? ' today' : '')
    headCell.textContent = `${names[i]} ${day.getDate()}`
    head.appendChild(headCell)
  }

  const allDay = state.events.filter((ev) => ev.all_day)
  head.appendChild(cell('div', 'allday-label'))
  for (let i = 0; i < 7; i++) {
    const day = addDays(first, i)
    const key = dateKey(day)
    const cellEl = document.createElement('div')
    cellEl.className = 'allday-cell'
    cellEl.dataset.date = key
    for (const ev of allDay) {
      if (!eventDayKeys(ev).includes(key)) continue
      const chip = document.createElement('button')
      chip.type = 'button'
      chip.className = 'chip'
      chip.dataset.id = ev.id
      chip.dataset.calendar = ev.calendar_id || ''
      chip.style.cssText = chipStyle(ev.calendar_id)
      chip.textContent = ev.summary
      chip.title = ev.summary
      cellEl.appendChild(chip)
    }
    head.appendChild(cellEl)
  }

  const body = document.createElement('div')
  body.className = 'week week-body'

  const labels = document.createElement('div')
  labels.className = 'hour-labels'
  for (let h = 0; h < 24; h++) {
    const row = document.createElement('div')
    row.className = 'hl'
    row.textContent = h ? `${pad(h)}:00` : ''
    labels.appendChild(row)
  }
  body.appendChild(labels)

  const dayColumns = []
  for (let i = 0; i < 7; i++) {
    const day = addDays(first, i)
    const col = document.createElement('div')
    col.className = 'day-col'
    col.dataset.date = dateKey(day)
    for (let h = 0; h < 24; h++) {
      const slot = document.createElement('div')
      slot.className = 'slot'
      slot.dataset.hour = String(h)
      col.appendChild(slot)
    }
    dayColumns.push(col)
    body.appendChild(col)
  }

  // Timed events, per day, with overlapping ones side by side.
  const HOUR = 44
  for (let i = 0; i < 7; i++) {
    const day = addDays(first, i)
    const dayStart = day.getTime()
    const dayEnd = addDays(day, 1).getTime()
    const items = []
    for (const ev of state.events) {
      if (ev.all_day) continue
      const s = new Date(ev.start).getTime()
      const e = new Date(ev.end).getTime()
      if (e <= dayStart || s >= dayEnd) continue
      const from = Math.max(s, dayStart)
      const to = Math.min(e, dayEnd)
      items.push({ ev, s: from, e: to, top: ((from - dayStart) / 3600000) * HOUR, height: Math.max(18, ((to - from) / 3600000) * HOUR - 2) })
    }
    items.sort((a, b) => a.s - b.s)
    const columnEnds = []
    for (const item of items) {
      let col = columnEnds.findIndex((end) => end <= item.s)
      if (col < 0) { col = columnEnds.length; columnEnds.push(0) }
      columnEnds[col] = item.e
      item.col = col
    }
    const columns = Math.max(1, columnEnds.length)
    for (const item of items) {
      const el = document.createElement('button')
      el.type = 'button'
      el.className = 'timed'
      el.dataset.id = item.ev.id
      el.dataset.calendar = item.ev.calendar_id || ''
      el.style.cssText = chipStyle(item.ev.calendar_id) +
        `;top:${Math.round(item.top)}px;height:${Math.round(item.height)}px;` +
        `left:calc(${(item.col / columns) * 100}% + 2px);width:calc(${100 / columns}% - 4px)`
      el.innerHTML = `<span class="t">${esc(timeLabel(item.ev))}</span>${esc(item.ev.summary)}`
      el.title = `${item.ev.summary}\n${timeLabel(item.ev)}`
      dayColumns[i].appendChild(el)
    }
  }

  root.appendChild(head)
  root.appendChild(body)
  // A week opens at the working day, not at midnight.
  requestAnimationFrame(() => {
    const scroller = $('view')
    if (scroller) scroller.scrollTop = Math.max(0, 8 * HOUR - 20)
  })
}

function renderAgenda(root) {
  const box = document.createElement('div')
  box.className = 'agenda'
  if (!state.events.length) {
    box.innerHTML = `<div class="empty">${esc(t('noEvents'))}</div>`
    root.appendChild(box)
    return
  }
  const groups = new Map()
  for (const ev of state.events) {
    const key = dateKey(eventStartDay(ev))
    if (!groups.has(key)) groups.set(key, [])
    groups.get(key).push(ev)
  }
  const keys = [...groups.keys()].sort()
  const todayKey = dateKey(new Date())
  for (const key of keys) {
    const group = document.createElement('div')
    group.className = 'group'
    const day = parseDateKey(key)
    const label = day.toLocaleDateString(locale(), { weekday: 'long', day: 'numeric', month: 'long' })
    group.innerHTML = `<div class="date">${esc(label)}${key === todayKey ? ' · ' + esc(t('today')) : ''}</div>`
    for (const ev of groups.get(key)) {
      const row = document.createElement('div')
      row.className = 'row'
      row.dataset.id = ev.id
      row.dataset.calendar = ev.calendar_id || ''
      const where = ev.location ? `<span class="where">· ${esc(ev.location)}</span>` : ''
      row.innerHTML = `<span class="when">${esc(ev.all_day ? t('allDayLabel') : timeLabel(ev))}</span>` +
        `<span class="what">${esc(ev.summary)} ${where}</span>` +
        `<span class="cal">${esc(ev.calendar_name || '')}</span>`
      group.appendChild(row)
    }
    box.appendChild(group)
  }
  root.appendChild(box)
}

function cell(tag, className) {
  const el = document.createElement(tag)
  if (className) el.className = className
  return el
}

function renderCalendars() {
  const box = $('calendars')
  box.innerHTML = ''
  for (const cal of state.calendars) {
    const label = document.createElement('label')
    label.className = 'cal-chip' + (cal.access_role === 'reader' || cal.access_role === 'freeBusyReader' ? ' muted' : '')
    label.title = `${cal.summary}\n${cal.id}\n${cal.access_role || ''}`
    const check = document.createElement('input')
    check.type = 'checkbox'
    check.checked = state.selected.has(cal.id)
    check.addEventListener('change', () => {
      if (check.checked) state.selected.add(cal.id)
      else state.selected.delete(cal.id)
      saveSelection()
      refresh()
    })
    const dot = document.createElement('span')
    dot.className = 'cal-dot'
    dot.style.background = cal.background_color || '#0d6efd'
    const name = document.createElement('span')
    name.textContent = cal.summary
    label.append(check, dot, name)
    box.appendChild(label)
  }
}

function showNotice(text, warn) {
  const notice = $('notice')
  notice.textContent = text
  notice.classList.toggle('warn', !!warn)
  notice.hidden = false
}

function hideNotice() {
  $('notice').hidden = true
}

// --- the editor ---------------------------------------------------------------------------------

let editing = null

function findEvent(id, calendar) {
  return state.events.find((ev) => ev.id === id && (ev.calendar_id || '') === (calendar || ''))
}

function openEditor(ev, defaults) {
  editing = ev || null
  $('editorError').hidden = true
  $('editorTitle').textContent = ev ? t('event') : t('newEventTitle')
  $('delete').hidden = !ev
  $('fCalendar').disabled = !!ev // moving between calendars is not in this version: the tool cannot do it
  $('repeatHint').hidden = !(ev && ev.recurring_event_id)
  $('repeatHint').textContent = t('repeatHint')
  $('linkHint').hidden = !(ev && ev.html_link)
  if (ev && ev.html_link) {
    $('linkHint').hidden = false
    $('htmlLink').href = ev.html_link
    $('htmlLink').textContent = t('inGoogle')
  }

  const allDay = ev ? !!ev.all_day : !!(defaults && defaults.allDay)
  $('fAllDay').checked = allDay

  let startDate, endDate, startTime, endTime
  if (ev) {
    if (ev.all_day) {
      startDate = ev.start
      // The API counts the end as the day after the last day; a person sees the last day.
      const end = parseDateKey(ev.end || ev.start)
      endDate = dateKey(addDays(end, -1))
      if (endDate < startDate) endDate = startDate
      startTime = '10:00'
      endTime = '11:00'
    } else {
      const s = new Date(ev.start)
      const e = new Date(ev.end)
      startDate = dateKey(s); startTime = `${pad(s.getHours())}:${pad(s.getMinutes())}`
      endDate = dateKey(e); endTime = `${pad(e.getHours())}:${pad(e.getMinutes())}`
    }
  } else {
    const date = (defaults && defaults.date) || dateKey(state.cursor)
    startDate = date; endDate = date
    startTime = (defaults && defaults.time) || '10:00'
    endTime = (defaults && defaults.time) ? nextHour(defaults.time) : '11:00'
  }

  $('fSummary').value = ev ? ev.summary : ''
  $('fStartDate').value = startDate
  $('fEndDate').value = endDate
  $('fStartTime').value = startTime
  $('fEndTime').value = endTime
  $('fLocation').value = ev ? ev.location || '' : ''
  $('fDescription').value = ev ? ev.description || '' : ''
  $('fAttendees').value = ev && ev.attendees ? ev.attendees.map((a) => a.email).join(', ') : ''

  const select = $('fCalendar')
  select.innerHTML = ''
  const chosen = ev ? ev.calendar_id : (defaults && defaults.calendar) || primarySelection()
  for (const cal of state.calendars) {
    const option = document.createElement('option')
    option.value = cal.id
    option.textContent = cal.summary
    option.selected = cal.id === chosen
    select.appendChild(option)
  }
  toggleTimes()
  $('editor').showModal()
  $('fSummary').focus()
}

function nextHour(time) {
  const [h] = time.split(':').map(Number)
  return `${pad(Math.min(23, h + 1))}:00`
}

function primarySelection() {
  const primary = state.calendars.find((c) => c.primary)
  if (primary && state.selected.has(primary.id)) return primary.id
  const first = state.calendars.find((c) => state.selected.has(c.id))
  return first ? first.id : (state.calendars[0] || {}).id || 'primary'
}

function toggleTimes() {
  const allDay = $('fAllDay').checked
  $('fStartTime').hidden = allDay
  $('fEndTime').hidden = allDay
}

/** splitEmails reads the guests field into the list the API wants. */
function splitEmails(raw) {
  return raw.split(/[,;\s]+/).map((s) => s.trim()).filter((s) => s.includes('@'))
}

async function saveEditor(event) {
  event.preventDefault()
  const error = $('editorError')
  error.hidden = true
  const fail = (message) => {
    error.textContent = message
    error.hidden = false
  }

  const allDay = $('fAllDay').checked
  const summary = $('fSummary').value.trim()
  if (!summary) return fail(t('needTitle'))

  let start, end
  if (allDay) {
    start = $('fStartDate').value
    end = $('fEndDate').value || start
    if (!start) return fail(t('badTimes'))
    if (end < start) return fail(t('endBeforeStart'))
  } else {
    const startDate = $('fStartDate').value
    const endDate = $('fEndDate').value
    const startTime = $('fStartTime').value || '00:00'
    const endTime = $('fEndTime').value || '00:00'
    if (!startDate || !endDate) return fail(t('badTimes'))
    start = rfc3339FromParts(startDate, startTime)
    end = rfc3339FromParts(endDate, endTime)
    if (new Date(end) <= new Date(start)) return fail(t('endBeforeStart'))
  }

  const payload = {
    calendar_id: $('fCalendar').value,
    summary,
    all_day: allDay,
    start,
    end,
    description: $('fDescription').value,
    location: $('fLocation').value,
    attendees: splitEmails($('fAttendees').value),
  }

  const save = $('save')
  save.disabled = true
  try {
    if (editing) {
      const query = `id=${encodeURIComponent(editing.id)}&calendar=${encodeURIComponent(editing.calendar_id || 'primary')}`
      await api('api/events?' + query, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
    } else {
      await api('api/events', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
    }
    $('editor').close()
    await refresh()
  } catch (e) {
    fail(e.message)
  } finally {
    save.disabled = false
  }
}

async function deleteEvent() {
  if (!editing) return
  if (!window.confirm(t('deleteQuestion'))) return
  const button = $('delete')
  button.disabled = true
  try {
    const query = `id=${encodeURIComponent(editing.id)}&calendar=${encodeURIComponent(editing.calendar_id || 'primary')}`
    await api('api/events?' + query, { method: 'DELETE' })
    $('editor').close()
    await refresh()
  } catch (e) {
    const error = $('editorError')
    error.textContent = e.message
    error.hidden = false
  } finally {
    button.disabled = false
  }
}

// --- loading and the bar ------------------------------------------------------------------------

async function refresh() {
  if (state.loading) return
  state.loading = true
  try {
    await loadEvents()
    render()
  } catch (e) {
    showNotice(e.message)
    state.events = []
    render()
  } finally {
    state.loading = false
  }
}

function shiftCursor(step) {
  if (state.view === 'month') {
    state.cursor = new Date(state.cursor.getFullYear(), state.cursor.getMonth() + step, 1)
  } else if (state.view === 'week') {
    state.cursor = addDays(startOfWeek(state.cursor), step * 7)
  } else {
    state.cursor = new Date(state.cursor.getFullYear(), state.cursor.getMonth() + step, 1)
  }
  refresh()
}

function setView(view) {
  state.view = view
  try {
    localStorage.setItem(VIEW_KEY, view)
  } catch (e) { /* private mode */ }
  refresh()
}

function bind() {
  $('prev').addEventListener('click', () => shiftCursor(-1))
  $('next').addEventListener('click', () => shiftCursor(1))
  $('today').addEventListener('click', () => {
    state.cursor = startOfDay(new Date())
    refresh()
  })
  for (const button of document.querySelectorAll('[data-view]')) {
    button.addEventListener('click', () => setView(button.dataset.view))
  }
  $('new').addEventListener('click', () => {
    const selected = state.selected.size === 1 ? [...state.selected][0] : undefined
    openEditor(null, { date: dateKey(state.cursor), calendar: selected })
  })
  $('watch').addEventListener('click', toggleReminders)
  $('refresh').addEventListener('click', async () => {
    try {
      await loadCalendars(true)
      await refresh()
    } catch (e) {
      showNotice(e.message)
    }
  })

  // A click on an event opens it; a click on an empty place of a day starts a new one there.
  $('view').addEventListener('click', (event) => {
    const target = event.target.closest('[data-id]')
    if (target) {
      const ev = findEvent(target.dataset.id, target.dataset.calendar)
      if (ev) openEditor(ev)
      return
    }
    const slot = event.target.closest('[data-date]')
    if (!slot) return
    const hour = event.target.closest('[data-hour]')
    const time = hour ? `${pad(Number(hour.dataset.hour))}:00` : undefined
    const selected = state.selected.size === 1 ? [...state.selected][0] : undefined
    openEditor(null, { date: slot.dataset.date, time, calendar: selected })
  })

  $('form').addEventListener('submit', saveEditor)
  $('cancel').addEventListener('click', () => $('editor').close())
  $('delete').addEventListener('click', deleteEvent)
  $('fAllDay').addEventListener('change', toggleTimes)
  $('editor').addEventListener('close', () => { editing = null })

  // The shell can change the theme or the language while the frame is open; the frame follows.
  window.addEventListener('storage', (event) => {
    if (event.key === 'anytext-theme') applyTheme()
    if (event.key === 'anytext.lang') {
      lang = storedLang()
      applyTexts()
      renderCalendars()
      render()
      if (editing) openEditor(editing)
    }
  })
}

/** applyTexts writes the words of the screen: the same dictionary, applied once. */
function applyTexts() {
  document.documentElement.lang = lang
  // The tab and the frame title: a direct link in an English browser must not read "Календарь".
  document.title = t('calendar')
  for (const el of document.querySelectorAll('[data-i18n]')) {
    el.textContent = t(el.dataset.i18n)
  }
  for (const el of document.querySelectorAll('[data-i18n-title]')) {
    el.title = t(el.dataset.i18nTitle)
  }
  $('repeatHint').textContent = t('repeatHint')
  $('htmlLink').textContent = t('inGoogle')
}

async function start() {
  // A direct link may name the language, the view and the day (…/ui/?view=week&date=2026-12-01).
  // Inside the shell's frame the page has no address of its own - its URL is the window's - so the
  // query is read only when the module is opened directly.
  const inFrame = window.self !== window.top
  const params = new URLSearchParams(window.location.search)
  if (!inFrame) {
    const qLang = params.get('lang')
    if (LANGS.includes(qLang)) lang = qLang
    const qView = params.get('view')
    if (['month', 'week', 'agenda'].includes(qView)) state.view = qView
    const qDate = params.get('date')
    if (/^\d{4}-\d{2}-\d{2}$/.test(qDate || '')) state.cursor = parseDateKey(qDate)
  }
  applyTheme()
  applyTexts()
  // The view is remembered between visits; a link that names one wins over the memory.
  if (inFrame || !params.get('view')) {
    try {
      state.view = localStorage.getItem(VIEW_KEY) || state.view
    } catch (e) { /* private mode */ }
  }
  bind()
  await loadStatus()
  await loadReminders()
  try {
    await loadCalendars(false)
  } catch (e) {
    showNotice(e.message)
  }
  await refresh()
}

start()
