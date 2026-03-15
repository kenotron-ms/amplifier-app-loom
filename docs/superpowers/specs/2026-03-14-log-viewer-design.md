# Log Viewer for Activity Runs

**Date:** 2026-03-14  
**Status:** Approved

## Overview

Add an enhanced inline log viewer to the Activity tab that supports both completed and live-streaming run output. When a user clicks "logs" on a run card, the card expands in-place to reveal a styled log panel. For runs that are currently executing, the panel streams output in real-time via Server-Sent Events (SSE).

## Goals

- View stdout+stderr output for completed runs in a readable, scrollable panel
- Watch live output from actively running jobs without waiting for completion
- Keep the activity list visible — no modal, no navigation away
- Copy log output with one click

## UI Design

### Run Card — Collapsed (default)

Unchanged from today except the "logs" link shows a `▾` chevron.

```
✓  build:release   2 minutes ago · 34s                    logs ▾
```

### Run Card — Expanded (completed run)

The card grows in-place to reveal the log panel below the header row:

```
✓  build:release   2 minutes ago · 34s                    hide ▴
┌─────────────────────────────────────────────────────────────────┐
│ stdout + stderr                                          copy   │
│─────────────────────────────────────────────────────────────────│
│ + Installing dependencies...                                    │
│ ✓ node_modules ready                                            │
│ ✓ TypeScript compiled (0 errors)                                │
│ ✓ Build complete in 34.2s                                       │
└─────────────────────────────────────────────────────────────────┘
```

- Max height: `240px`, scrollable
- Font: monospace, `11px`
- Dark background (`#0d1117`), light text (`#c9d1d9`)
- Copy button copies full content to clipboard

### Run Card — Expanded (live/running)

Running jobs get a blue accent border and a `● live` badge. The panel auto-expands on load (no click required for running jobs). Output auto-scrolls to the bottom as chunks arrive.

```
●  sync:data   started 12s ago                         ● live  hide ▴
┌─────────────────────────────────────────────────────────────────┐
│ streaming output                                         copy   │
│─────────────────────────────────────────────────────────────────│
│ ✓ Connected to database                                         │
│ → Fetching records (batch 1/10)...                              │
│ ✓ Batch 1 complete (1,240 rows)                                 │
│ → Fetching records (batch 2/10)... ▌                            │
└─────────────────────────────────────────────────────────────────┘
```

- Blue border (`#1a3a5c`) and glow on the card while running
- "streaming output" label instead of "stdout + stderr"
- Cursor blink (`▌`) appended to the last line of the `<pre>` via a `<span id="cursor-{id}">▌</span>` element; toggled visible/hidden by a CSS `@keyframes blink` animation on `.log-cursor`; removed entirely when the SSE `done` event fires
- When the run completes, the SSE connection closes, the live badge disappears, the blue border is removed, and the label changes to "stdout + stderr"

## Backend Design

### New SSE Endpoint

```
GET /api/runs/{id}/stream
```

**Handler logic:**
1. Load the run from the store. Return 404 if not found.
2. Set SSE response headers (`Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`).
3. If `run.Status` is not `running`: emit the stored `run.Output` as a single `data:` chunk event, emit `event: done`, flush, and return.
4. If `run.Status` is `running`: call `broadcaster.Subscribe(id)`. If `Subscribe` returns `done=true` (the run completed between step 1 and now — a race condition at run boundary), reload the run from the store to get the final `Output` and proceed as step 3. Otherwise, stream the buffered snapshot first, then loop over `select { case chunk, ok := <-ch; case <-r.Context().Done() }`. On `r.Context().Done()` (client disconnect), call `broadcaster.Unsubscribe(id, ch)` and return — this prevents the stale channel from remaining in the broadcaster. When `ok` is false (channel closed by `Complete`), emit `event: done` and return.

**SSE event format:**
```
data: {"chunk":"output text here\n"}

event: done
data: {"status":"success","started_at":"2026-03-14T22:00:00Z","ended_at":"2026-03-14T22:00:34Z"}
```

Possible `status` values in the `done` payload: `success`, `failed`, `timeout`. `started_at` and `ended_at` are ISO 8601 strings (matching the existing `JobRun` field format) so `finalizeRunCard` can immediately update the elapsed time display without waiting for the next `loadAll()` poll.

### In-Memory Output Broadcaster

New component: `internal/scheduler/broadcaster.go`

```go
type Broadcaster struct { ... }

func (b *Broadcaster) Register(runID string)
func (b *Broadcaster) Write(runID, chunk string)       // called by executors
func (b *Broadcaster) Subscribe(runID string) (buffered []string, ch chan string, done bool)
func (b *Broadcaster) Unsubscribe(runID string, ch chan string)
func (b *Broadcaster) Complete(runID string)           // signals done, closes channels
func (b *Broadcaster) Remove(runID string)             // cleanup after all subscribers gone
```

- `Register` is called at run start. `Complete` is called by `runner.go` **after** the DB write commits — this ordering guarantee ensures that any SSE client receiving `event: done` and then immediately issuing a `GET /api/runs/{id}` will find a fully-persisted run record with final output.
- `Complete` closes all active subscriber channels (causing the SSE handler's `select` loop to exit naturally via `ok=false`) and schedules `Remove` via `time.AfterFunc(60*time.Second, func() { b.Remove(runID) })`. If a subscriber is still draining at T+60s, `Remove` is a no-op for that run (it checks subscriber count before deleting; if any remain, it reschedules itself for another 30 seconds).
- `Write` appends to an in-memory `[]string` buffer AND fans out to all active subscriber channels using a **non-blocking send** (`select { case ch <- chunk; default: }`). A full channel (slow SSE client) causes that client to miss the chunk silently — this protects the executor goroutine from stalling. The buffer is always updated regardless of subscriber state, so late `Subscribe` calls still receive complete history. **Invariant:** callers must call `Register` before any `Write`. Calls to `Write` for an unregistered or already-`Remove`d `runID` are silently dropped.
- `Subscribe` returns a bidirectional `chan string` (capacity 256) so it can be passed directly to `Unsubscribe`. Returns `done=true` (and nil channel) if the run is already complete or if the `runID` is unknown — both cases tell the SSE handler to fall back to stored output.
- `Unsubscribe` removes the channel from the subscriber list and drains any pending items; called by the SSE handler on both normal close and client disconnect (`r.Context().Done()`). Calling `Unsubscribe` with an unknown `runID` or already-removed channel is a no-op.
- Thread-safe via `sync.RWMutex`

The `Broadcaster` instance lives on the `Daemon` struct, passed into both the scheduler (for writes) and the API server (for SSE reads).

### Executor Changes

Each executor (`exec_shell.go`, `exec_claude_code.go`, `exec_amplifier.go`) currently uses `cmd.CombinedOutput()` which blocks until completion. This needs to switch to streaming capture:

1. Replace `cmd.CombinedOutput()` with `cmd.StdoutPipe()` + `cmd.StderrPipe()`
2. Read from pipes in a goroutine, appending each chunk to a local accumulator string
3. **64 KB cap is enforced in the accumulation loop only**: once `len(accumulated) >= 64*1024`, stop appending to the accumulation buffer. The broadcaster is intentionally uncapped — `broadcaster.Write` is called for every chunk regardless of the accumulation cap, so live viewers always see full output.
4. Call `broadcaster.Write(runID, chunk)` unconditionally on every chunk, then conditionally append to the accumulation buffer if under the cap
5. Return the accumulated (capped) string as before — same as `capOutput()` today

The executor interface signature does not change. The broadcaster is injected via the `Runner` struct.

### API Server

- Add route: `GET /api/runs/{id}/stream` → `handlers_stream.go`
- `Server` struct receives the `Broadcaster` reference at construction time

### Store / DB

No changes. Output is still written to the `runs` BoltDB bucket at run completion, exactly as today.

The existing `GET /api/runs?limit=30` list endpoint already returns full `JobRun` records including the `output` field (confirmed by reviewing `handlers_runs.go` and `store.ListRecentRuns`). Completed card templates can render `{escapedOutput}` inline without any API changes.

## Frontend Design

### `renderRunCard` changes (`web/app.js`)

Replace the existing simple toggle with the new log panel structure. There are three card states; the three HTML blocks below are the canonical templates (they supersede any ASCII art in the UI Design section above). The escaping helper `esc()` already exists in `app.js` and is used throughout the current code — use it for all output fields.

```html
<!-- Success run — green checkmark, panel hidden by default -->
<div class="run-card" id="run-{id}">
  <div class="run-header">
    <span class="run-status-icon" id="status-icon-{id}">✓</span>
    <span class="run-name">{name}</span>
    <span class="run-time" id="run-time-{id}">{timeAgo} · {duration}</span>
    <a class="log-toggle" href="#" onclick="toggleLog('{id}', this); return false;">logs ▾</a>
  </div>
  <div class="log-panel hidden" id="log-{id}">
    <div class="log-toolbar">
      <span class="log-label" id="log-label-{id}">stdout + stderr</span>
      <button class="log-copy-btn" onclick="copyLog('{id}')">copy</button>
    </div>
    <pre class="log-output" id="logout-{id}">{esc(run.output)}</pre>
  </div>
</div>

<!-- Failed / timeout run — same structure as completed, with red icon and run-failed class -->
<div class="run-card run-failed" id="run-{id}">
  <div class="run-header">
    <span class="run-status-icon" id="status-icon-{id}" style="color:#f44336">✕</span>
    <span class="run-name">{name}</span>
    <span class="run-time" id="run-time-{id}">{timeAgo} · {duration}</span>
    <a class="log-toggle" href="#" onclick="toggleLog('{id}', this); return false;">logs ▾</a>
  </div>
  <div class="log-panel hidden" id="log-{id}">
    <div class="log-toolbar">
      <span class="log-label" id="log-label-{id}">stdout + stderr</span>
      <button class="log-copy-btn" onclick="copyLog('{id}')">copy</button>
    </div>
    <pre class="log-output" id="logout-{id}">{esc(run.output)}</pre>
  </div>
</div>

<!-- Running job — panel open by default, no output yet -->
<div class="run-card live" id="run-{id}">
  <div class="run-header">
    <span class="run-status-icon" id="status-icon-{id}" style="color:#2196F3">●</span>
    <span class="run-name">{name}</span>
    <span class="run-time" id="run-time-{id}">started {timeAgo}</span>
    <span class="live-badge" id="live-badge-{id}">● live</span>
    <a class="log-toggle" href="#" onclick="toggleLog('{id}', this); return false;">hide ▴</a>
  </div>
  <div class="log-panel" id="log-{id}">
    <div class="log-toolbar">
      <span class="log-label" id="log-label-{id}">streaming output</span>
      <button class="log-copy-btn" onclick="copyLog('{id}')">copy</button>
    </div>
    <pre class="log-output" id="logout-{id}"><span id="cursor-{id}" class="log-cursor">▌</span></pre>
  </div>
</div>
```

**Panel visibility:** `.log-panel.hidden { display: none }` (added to CSS). `toggleLog` adds/removes the `hidden` class and flips the link text between `"logs ▾"` and `"hide ▴"`:

```js
function toggleLog(id, link) {
  const panel = document.getElementById(`log-${id}`);
  const hidden = panel.classList.toggle('hidden');
  link.textContent = hidden ? 'logs ▾' : 'hide ▴';
}
```

Running job panels are rendered without `hidden` (open by default). Completed job panels are rendered with `hidden` (collapsed by default, matching existing behaviour).

### SSE subscription

**Call site:** `openLiveLog` is called **only from `loadAll()`**, not from `renderRunCard`. After `loadAll()` renders or updates a card, it checks: if `run.status === "running"` and `liveSources[run.id]` is absent, it calls `openLiveLog(run.id)`. If `liveSources[run.id]` already exists for a running card, `loadAll()` skips updating that card's log panel DOM entirely (the `#log-{id}` element is left untouched), preserving accumulated live output.

```js
function openLiveLog(runId) {
  const pre = document.getElementById(`logout-${runId}`);
  const cursor = document.getElementById(`cursor-${runId}`);
  const src = new EventSource(`/api/runs/${runId}/stream`);

  src.onmessage = e => {
    const d = JSON.parse(e.data);
    if (d.chunk) {
      const atBottom = pre.scrollHeight - pre.scrollTop <= pre.clientHeight + 4;
      // Insert text node before cursor span to preserve cursor element
      pre.insertBefore(document.createTextNode(d.chunk), cursor);
      if (atBottom) pre.scrollTop = pre.scrollHeight;
    }
  };

  src.addEventListener('done', e => {
    src.close();
    delete liveSources[runId];
    const { status, started_at, ended_at } = JSON.parse(e.data);
    finalizeRunCard(runId, status, started_at, ended_at);
  });

  src.onerror = () => {
    src.close(); // prevent EventSource auto-reconnect
    delete liveSources[runId];
    failedSources.add(runId); // guard: prevent loadAll() from reconnecting after error
    finalizeRunCard(runId, 'failed');
  };

  liveSources[runId] = src;
}
```

- `liveSources` is a module-level `{}` map keyed by run ID. Entries are deleted on `done` or `onerror`.
- `failedSources` is a module-level `Set`. Run IDs are added on `onerror` and never removed (within the page session). The `loadAll()` loop checks `failedSources.has(run.id)` before calling `openLiveLog` — a run that errored via SSE will not be reconnected even if the server still reports it as `running`.
- Appending text via `insertBefore(createTextNode(...), cursor)` keeps the cursor `<span>` as a child element, avoiding the `textContent` clobber issue.
- Auto-scroll only fires if the user is already near the bottom, so manual upward scrolling is not interrupted.

### `finalizeRunCard(runId, status, startedAt, endedAt)`

Called when SSE closes (run complete or connection error). `status` is one of `success`, `failed`, `timeout`. For `onerror` calls, `startedAt` and `endedAt` may be omitted (time update is skipped). Performs these exact DOM operations, all with null-checks (`?.`) since `onerror` may call this a second time after elements have already been removed:

1. Remove class `live` from `#run-{runId}`
2. Set `#status-icon-{runId}` text to `✓` (success) or `✕` (failed/timeout); set its color to `#4CAF50` or `#f44336` accordingly
3. Remove element `#live-badge-{runId}` from the DOM (if present)
4. Set `#log-label-{runId}` text content to `"stdout + stderr"`
5. Remove element `#cursor-{runId}` from the DOM (if present)
6. If `status !== "success"`: add class `run-failed` to `#run-{runId}`
7. If `startedAt` and `endedAt` are provided: set `#run-time-{runId}` to `"${timeAgo(startedAt)} · ${duration(startedAt, endedAt)}"` so the card immediately shows correct timing without waiting for the next `loadAll()` poll

### `loadAll()` coexistence with SSE

`loadAll()` calls `GET /api/runs?limit=30` every 3 seconds. The existing implementation builds a full `innerHTML` string for the run list. The updated approach replaces the inner loop with per-card logic:

```js
async function loadAll() {
  // API returns newest-first. Iterate oldest-first so each 'afterbegin' prepend
  // leaves the newest card at the top of the list.
  const runs = await fetch('/api/runs?limit=30').then(r => r.json());
  const list = document.getElementById('run-list');

  for (const run of [...runs].reverse()) {
    const existing = document.getElementById(`run-${run.id}`);
    if (existing) {
      // Any existing card: update elapsed time and (for non-live cards) sync status icon
      // in-place. Never replace outerHTML — that collapses user-expanded log panels.
      const timeEl = document.getElementById(`run-time-${run.id}`);
      if (timeEl) timeEl.textContent = run.status === 'running'
        ? `started ${timeAgo(run.started_at)}`
        : `${timeAgo(run.started_at)} · ${duration(run.started_at, run.ended_at)}`;

      if (!liveSources[run.id] && run.status !== 'running') {
        // Correct status icon in case the card was incorrectly marked failed by onerror
        // (e.g., after a transient network blip that resolved before the run ended)
        const icon = document.getElementById(`status-icon-${run.id}`);
        if (icon) {
          icon.textContent = run.status === 'success' ? '✓' : '✕';
          icon.style.color = run.status === 'success' ? '#4CAF50' : '#f44336';
        }
        const card = document.getElementById(`run-${run.id}`);
        if (card) {
          card.classList.remove('run-failed');
          if (run.status !== 'success') card.classList.add('run-failed');
        }
      }

      if (!liveSources[run.id] && !failedSources.has(run.id) && run.status === 'running') openLiveLog(run.id);
      continue;
    }

    // New card: prepend to list and start SSE if running.
    // The loop iterates over a reversed copy of the API response (oldest → newest)
    // so each 'afterbegin' prepend keeps the newest card at the top of the list.
    list.insertAdjacentHTML('afterbegin', renderRunCard(run));
    if (run.status === 'running') openLiveLog(run.id);
  }

  // Remove cards no longer in the response; close any live SSE connection first
  list.querySelectorAll('.run-card').forEach(el => {
    if (!runs.find(r => `run-${r.id}` === el.id)) {
      const runId = el.id.replace('run-', '');
      if (liveSources[runId]) { liveSources[runId].close(); delete liveSources[runId]; }
      el.remove();
    }
  });
}
```

**Key rule:** any card whose `id` already exists in the DOM — live or completed — is skipped for full re-render. This preserves both SSE log state and user-toggled panel state (e.g., a user who clicked "logs ▾" to open a completed card's panel won't have it collapse on the next poll). Only the elapsed time field (`#run-time-{id}`) is updated in-place for all existing cards.

`renderRunCard(run)` returns the canonical HTML string for a card (completed or running template as above). For running cards, it renders the panel open with an empty `<pre>` + cursor span; `openLiveLog` then streams content into it.

### Copy button

```js
function copyLog(runId) {
  const pre = document.getElementById(`logout-${runId}`);
  // Collect text from all child nodes except the cursor span, to avoid copying the blinking ▌
  const text = Array.from(pre.childNodes)
    .filter(n => !(n.nodeType === Node.ELEMENT_NODE && n.id === `cursor-${runId}`))
    .map(n => n.textContent)
    .join('');
  navigator.clipboard.writeText(text).catch(() => {}); // silent failure on HTTP / permission denial
}
```

### CSS additions (`web/style.css`)

```css
.run-card.live          { border-color: #1a3a5c; box-shadow: 0 0 0 1px rgba(33,150,243,0.2); }
.run-card.run-failed    { border-left: 2px solid #f44336; }
.log-panel              { border-top: 1px solid #222; }
.log-panel.hidden       { display: none; }
.log-toolbar            { display: flex; justify-content: space-between; padding: 4px 12px; background: #111827; }
.log-label              { color: #555; font-size: 11px; font-family: monospace; }
.log-copy-btn           { background: none; border: none; color: #4a9eff; font-size: 11px; cursor: pointer; padding: 0; }
.log-output             { margin: 0; padding: 12px; background: #0d1117; color: #c9d1d9;
                          font-size: 11px; font-family: 'SF Mono', monospace; line-height: 1.5;
                          max-height: 240px; overflow-y: auto; }
.live-badge             { display: inline-flex; align-items: center; gap: 4px; color: #2196F3; font-size: 11px; }
.log-cursor             { animation: blink 1s step-end infinite; }
@keyframes blink        { 0%, 100% { opacity: 1; } 50% { opacity: 0; } }
```

## File Inventory

| File | Change |
|---|---|
| `internal/scheduler/broadcaster.go` | **New** — output broadcaster |
| `internal/api/handlers_stream.go` | **New** — SSE endpoint handler |
| `internal/api/server.go` | Add route + broadcaster field |
| `internal/service/daemon.go` | Construct broadcaster, wire to scheduler + API |
| `internal/scheduler/runner.go` | Accept broadcaster, call Register/Complete |
| `internal/scheduler/exec_shell.go` | Switch to streaming pipe capture |
| `internal/scheduler/exec_claude_code.go` | Switch to streaming pipe capture |
| `internal/scheduler/exec_amplifier.go` | Switch to streaming pipe capture |
| `web/app.js` | New log panel, SSE subscription, copy button |
| `web/style.css` | New log panel styles |

## Out of Scope

- Log search / filtering
- Log download (file save)
- Persisting partial output to DB during run (only final output is stored)
- Log rotation or size limits beyond the existing 64 KB cap
- Daemon restart recovery: if the daemon restarts while a job is running, the in-memory broadcaster is lost; the SSE client will receive an `onerror` event and `finalizeRunCard` will show a failure badge even if the run ultimately succeeded (acceptable degradation)
- Transient network error recovery: a brief disconnect (e.g. laptop sleep) triggers `onerror`, which permanently marks the card failed in the UI even if the underlying run completes successfully; the 3-second `loadAll()` poll will eventually correct the card's final status
