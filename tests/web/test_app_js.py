"""
Tests for web/app.js — validates renderRunCard variants, toggleLog, copyLog,
and removal of toggleRunLog.
"""
import re
import pytest
from pathlib import Path

APP_JS_PATH = Path(__file__).parent.parent.parent / "web" / "app.js"


@pytest.fixture(scope="module")
def js_text():
    return APP_JS_PATH.read_text()


# ---------------------------------------------------------------------------
# renderRunCard — running variant
# ---------------------------------------------------------------------------

class TestRenderRunCardRunning:
    def test_running_card_has_live_class(self, js_text):
        """Running card must use class 'run-card live'."""
        assert 'run-card live' in js_text, \
            "renderRunCard must produce a card with class 'run-card live' for running status"

    def test_running_card_id(self, js_text):
        """Running card must have id='run-${run.id}'."""
        assert 'id="run-${run.id}"' in js_text or "id=`run-${run.id}`" in js_text, \
            "Running card must have id set to run-${run.id}"

    def test_running_card_status_icon_id(self, js_text):
        """Running card status icon must have id='status-icon-${run.id}'."""
        assert 'status-icon-${run.id}' in js_text, \
            "Running card must have status icon with id status-icon-${run.id}"

    def test_running_card_status_icon_blue_color(self, js_text):
        """Running card status icon must use blue color #2196F3."""
        assert '#2196F3' in js_text, \
            "Running card status icon must use blue color #2196F3"

    def test_running_card_run_time_id(self, js_text):
        """Running card run-time element must have id='run-time-${run.id}'."""
        assert 'run-time-${run.id}' in js_text, \
            "Running card must have run-time element with id run-time-${run.id}"

    def test_running_card_started_text(self, js_text):
        """Running card run-time must show 'started ...'."""
        assert 'started ${timeAgo(' in js_text or "started ${ timeAgo(" in js_text, \
            "Running card run-time must show 'started ' + timeAgo(run.startedAt)"

    def test_running_card_live_badge_id(self, js_text):
        """Running card must have live-badge with id='live-badge-${run.id}'."""
        assert 'live-badge-${run.id}' in js_text, \
            "Running card must have live-badge with id live-badge-${run.id}"

    def test_running_card_live_badge_text(self, js_text):
        """Running card live-badge must show '● live'."""
        assert '● live' in js_text or '\u25cf live' in js_text, \
            "Running card live-badge must display '● live'"

    def test_running_card_log_toggle_calls_toggleLog(self, js_text):
        """Running card log-toggle must call toggleLog."""
        assert "toggleLog('${run.id}'" in js_text or 'toggleLog(`${run.id}`' in js_text or "toggleLog('${run.id}'," in js_text, \
            "Running card log-toggle must call toggleLog with run.id"

    def test_running_card_log_toggle_text_hide(self, js_text):
        """Running card log-toggle initial text must be 'hide ▴'."""
        assert 'hide \u25b4' in js_text, \
            "Running card log-toggle must have initial text 'hide ▴'"

    def test_running_card_log_panel_not_hidden(self, js_text):
        """Running card log panel must NOT have class 'hidden' initially."""
        # The running card log panel should use id log-${run.id} without 'hidden' class
        # We verify that the log-panel for running status does not include 'hidden'
        # Find the running branch of renderRunCard
        running_branch = _extract_running_branch(js_text)
        assert running_branch is not None, "Could not extract running card branch from renderRunCard"
        assert 'log-panel hidden' not in running_branch, \
            "Running card log panel must NOT have 'hidden' class"
        assert 'log-panel' in running_branch, \
            "Running card must have a log-panel"

    def test_running_card_log_panel_id(self, js_text):
        """Running card log panel must have id='log-${run.id}'."""
        assert 'log-${run.id}' in js_text, \
            "Running card must have log panel with id log-${run.id}"

    def test_running_card_log_label_streaming(self, js_text):
        """Running card log toolbar label must say 'streaming output'."""
        assert 'streaming output' in js_text, \
            "Running card log toolbar must show 'streaming output'"

    def test_running_card_log_label_id(self, js_text):
        """Running card log label must have id='log-label-${run.id}'."""
        assert 'log-label-${run.id}' in js_text, \
            "Running card must have log label with id log-label-${run.id}"

    def test_running_card_copy_button_calls_copyLog(self, js_text):
        """Running card copy button must call copyLog."""
        assert "copyLog('${run.id}')" in js_text or 'copyLog(`${run.id}`)' in js_text, \
            "Running card must have a copy button calling copyLog"

    def test_running_card_log_output_id(self, js_text):
        """Running card log pre must have id='logout-${run.id}'."""
        assert 'logout-${run.id}' in js_text, \
            "Running card log pre must have id logout-${run.id}"

    def test_running_card_cursor_span_id(self, js_text):
        """Running card must have cursor span with id='cursor-${run.id}'."""
        assert 'cursor-${run.id}' in js_text, \
            "Running card must have cursor span with id cursor-${run.id}"

    def test_running_card_cursor_span_class(self, js_text):
        """Running card must have cursor span with class='log-cursor'."""
        assert 'log-cursor' in js_text, \
            "Running card cursor span must have class log-cursor"

    def test_running_card_cursor_char(self, js_text):
        """Running card cursor must contain the block cursor character ▌."""
        assert '\u258c' in js_text, \
            "Running card cursor span must contain '▌' character"


# ---------------------------------------------------------------------------
# renderRunCard — completed/success variant
# ---------------------------------------------------------------------------

class TestRenderRunCardCompleted:
    def test_success_icon(self, js_text):
        """Success card must use ✓ icon."""
        assert '\u2713' in js_text, "renderRunCard must use ✓ icon for success"

    def test_success_color_green(self, js_text):
        """Success card must use green color."""
        assert 'var(--green)' in js_text, "Success card must use var(--green) color"

    def test_completed_log_panel_hidden(self, js_text):
        """Completed card log panel must have class 'hidden'."""
        assert 'log-panel hidden' in js_text, \
            "Completed/failed card log panel must have class 'hidden'"

    def test_completed_log_toggle_text_logs(self, js_text):
        """Completed card log-toggle initial text must be 'logs ▾'."""
        assert 'logs \u25be' in js_text, \
            "Completed card log-toggle must have text 'logs ▾'"

    def test_completed_duration_shown(self, js_text):
        """Completed card run-time must show duration via durationMs."""
        assert 'durationMs(' in js_text, \
            "Completed card must show duration using durationMs()"

    def test_completed_log_output_has_run_output(self, js_text):
        """Completed card log pre must contain esc(run.output)."""
        assert "esc(run.output" in js_text, \
            "Completed card log pre must contain esc(run.output || '')"


# ---------------------------------------------------------------------------
# renderRunCard — failed variant
# ---------------------------------------------------------------------------

class TestRenderRunCardFailed:
    def test_failed_icon(self, js_text):
        """Failed card must use ✕ icon."""
        assert '\u2715' in js_text, "renderRunCard must use ✕ icon for failure"

    def test_failed_color_red(self, js_text):
        """Failed card must use red color."""
        assert 'var(--red)' in js_text, "Failed card must use var(--red) color"

    def test_failed_class_applied(self, js_text):
        """Failed card must have 'run-failed' class."""
        assert 'run-failed' in js_text, \
            "renderRunCard must apply 'run-failed' class for non-success runs"


# ---------------------------------------------------------------------------
# toggleLog
# ---------------------------------------------------------------------------

class TestToggleLog:
    def test_toggleLog_exists(self, js_text):
        """toggleLog function must exist."""
        assert re.search(r'function\s+toggleLog\s*\(', js_text), \
            "toggleLog function must be defined"

    def test_toggleLog_gets_panel_by_log_id(self, js_text):
        """toggleLog must look up element by 'log-' + id."""
        assert re.search(r'getElementById\s*\(\s*[`\'"]log-\$\{id\}', js_text), \
            "toggleLog must get panel by id 'log-${id}'"

    def test_toggleLog_toggles_hidden(self, js_text):
        """toggleLog must toggle 'hidden' class on the panel."""
        assert re.search(r'classList\.toggle\s*\(\s*[\'"]hidden[\'"]\s*\)', js_text), \
            "toggleLog must use classList.toggle('hidden')"

    def test_toggleLog_sets_text_logs_when_hidden(self, js_text):
        """toggleLog must set link text to 'logs ▾' when panel is hidden."""
        assert 'logs \u25be' in js_text, \
            "toggleLog must use 'logs ▾' text when panel is hidden"

    def test_toggleLog_sets_text_hide_when_visible(self, js_text):
        """toggleLog must set link text to 'hide ▴' when panel is visible."""
        assert 'hide \u25b4' in js_text, \
            "toggleLog must use 'hide ▴' text when panel is visible"


# ---------------------------------------------------------------------------
# copyLog
# ---------------------------------------------------------------------------

class TestCopyLog:
    def test_copyLog_exists(self, js_text):
        """copyLog function must exist."""
        assert re.search(r'function\s+copyLog\s*\(', js_text), \
            "copyLog function must be defined"

    def test_copyLog_gets_pre_by_logout_id(self, js_text):
        """copyLog must look up pre by 'logout-' + runId."""
        assert re.search(r'getElementById\s*\(\s*[`\'"]logout-\$\{runId\}', js_text), \
            "copyLog must get pre by id 'logout-${runId}'"

    def test_copyLog_filters_cursor_by_id(self, js_text):
        """copyLog must filter out the cursor span by checking n.id against cursor-${runId}."""
        assert 'cursor-${runId}' in js_text, \
            "copyLog must filter nodes by checking n.id !== 'cursor-${runId}'"

    def test_copyLog_uses_childNodes(self, js_text):
        """copyLog must iterate over childNodes."""
        assert 'childNodes' in js_text, \
            "copyLog must collect text from childNodes"

    def test_copyLog_uses_clipboard(self, js_text):
        """copyLog must use navigator.clipboard.writeText."""
        assert 'navigator.clipboard.writeText' in js_text, \
            "copyLog must use navigator.clipboard.writeText"

    def test_copyLog_silent_catch(self, js_text):
        """copyLog must silently catch clipboard errors with .catch(() => {})."""
        assert re.search(r'\.catch\s*\(\s*\(\s*\)\s*=>\s*\{\s*\}\s*\)', js_text), \
            "copyLog must silently catch clipboard errors"


# ---------------------------------------------------------------------------
# toggleRunLog — must be REMOVED
# ---------------------------------------------------------------------------

class TestToggleRunLogRemoved:
    def test_toggleRunLog_function_removed(self, js_text):
        """Old toggleRunLog function must be removed."""
        assert not re.search(r'function\s+toggleRunLog\s*\(', js_text), \
            "toggleRunLog must be removed (superseded by toggleLog)"

    def test_toggleRunLog_not_called(self, js_text):
        """toggleRunLog must not be called anywhere."""
        assert 'toggleRunLog' not in js_text, \
            "toggleRunLog must not appear anywhere in app.js"


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _extract_running_branch(js_text):
    """
    Try to extract the 'running' branch of renderRunCard.
    Finds renderRunCard first, then looks for the status === 'running' check
    within that function, returning up to 800 chars from that point.
    """
    # Find renderRunCard function start first
    func_match = re.search(r'function\s+renderRunCard\s*\(', js_text)
    if not func_match:
        return None
    func_start = func_match.start()
    # Search for status === 'running' only within renderRunCard's body
    m = re.search(r"status\s*===\s*['\"]running['\"]", js_text[func_start:])
    if not m:
        return None
    start = func_start + m.start()
    snippet = js_text[start:start + 800]
    return snippet


# ---------------------------------------------------------------------------
# Task 3: Incremental loadRuns — renderRuns removed, new incremental logic
# ---------------------------------------------------------------------------

class TestRenderRunsRemoved:
    def test_renderRuns_function_removed(self, js_text):
        """Old renderRuns function must be removed (replaced by incremental loadRuns)."""
        assert not re.search(r'function\s+renderRuns\s*\(', js_text), \
            "renderRuns function must be removed — incremental loadRuns replaces it"

    def test_renderRuns_not_called(self, js_text):
        """renderRuns must not be called anywhere in the code."""
        assert 'renderRuns(' not in js_text, \
            "renderRuns() must not be called — new loadRuns handles rendering incrementally"


class TestLiveSourcesState:
    def test_liveSources_declared(self, js_text):
        """liveSources module-level object must be declared before loadRuns."""
        assert re.search(r'const\s+liveSources\s*=\s*\{\}', js_text), \
            "liveSources = {} must be declared as module-level state before loadRuns"

    def test_failedSources_declared(self, js_text):
        """failedSources module-level Set must be declared before loadRuns."""
        assert re.search(r'const\s+failedSources\s*=\s*new\s+Set\s*\(\s*\)', js_text), \
            "failedSources = new Set() must be declared as module-level state before loadRuns"

    def test_liveSources_before_loadRuns(self, js_text):
        """liveSources declaration must appear before async function loadRuns."""
        live_match = re.search(r'const\s+liveSources\s*=\s*\{\}', js_text)
        load_match = re.search(r'async\s+function\s+loadRuns\s*\(', js_text)
        assert live_match and load_match, \
            "Both liveSources and loadRuns must exist"
        assert live_match.start() < load_match.start(), \
            "liveSources must be declared before async function loadRuns"


class TestIncrementalLoadRuns:
    def test_loadRuns_uses_insertAdjacentHTML(self, js_text):
        """loadRuns must use insertAdjacentHTML('afterbegin') instead of innerHTML."""
        assert "insertAdjacentHTML('afterbegin'" in js_text or \
               'insertAdjacentHTML("afterbegin"' in js_text, \
            "loadRuns must use insertAdjacentHTML('afterbegin', ...) for incremental card insertion"

    def test_loadRuns_iterates_reverse(self, js_text):
        """loadRuns must iterate runs in reverse (oldest-first) via [...runs].reverse()."""
        assert re.search(r'\[\s*\.\.\.\s*runs\s*\]\s*\.reverse\s*\(\s*\)', js_text), \
            "loadRuns must iterate [...runs].reverse() to insert oldest first via afterbegin"

    def test_loadRuns_checks_existing_card_by_id(self, js_text):
        """loadRuns must look up existing cards by id 'run-${run.id}'."""
        assert re.search(r"getElementById\s*\(\s*[`'\"]run-\$\{run\.id\}", js_text), \
            "loadRuns must check for existing card via getElementById('run-${run.id}')"

    def test_loadRuns_updates_run_time_element(self, js_text):
        """loadRuns must update the run-time-${run.id} element in-place."""
        assert re.search(r"getElementById\s*\(\s*[`'\"]run-time-\$\{run\.id\}", js_text), \
            "loadRuns must update run-time element via getElementById('run-time-${run.id}')"

    def test_loadRuns_calls_openLiveLog(self, js_text):
        """loadRuns must call openLiveLog(run.id) for running runs."""
        assert re.search(r'openLiveLog\s*\(\s*run\.id\s*\)', js_text), \
            "loadRuns must call openLiveLog(run.id) for running runs"

    def test_loadRuns_removes_stale_cards(self, js_text):
        """loadRuns must remove cards for runs no longer in API response."""
        assert re.search(r'\.remove\s*\(\s*\)', js_text), \
            "loadRuns must call .remove() to remove stale run cards"

    def test_loadRuns_closes_stale_sse(self, js_text):
        """loadRuns must close SSE connections for stale runs before removing the card."""
        assert re.search(r'liveSources\s*\[.*\]\s*\.close\s*\(\s*\)', js_text), \
            "loadRuns must call liveSources[runId].close() when removing stale cards"

    def test_loadRuns_empty_state_conditional(self, js_text):
        """loadRuns must only show empty state if no .run-card exists in list."""
        assert re.search(r'querySelector\s*\(\s*[`\'"]\.run-card[`\'"]', js_text), \
            "loadRuns must use querySelector('.run-card') to avoid overwriting cards with empty state"

    def test_loadRuns_removes_empty_placeholder(self, js_text):
        """loadRuns must remove stale .empty placeholder when runs appear."""
        assert re.search(r'querySelector\s*\(\s*[`\'"]\.empty[`\'"]', js_text), \
            "loadRuns must remove stale .empty placeholder via querySelector('.empty')"

    def test_loadRuns_checks_failedSources(self, js_text):
        """loadRuns must skip reconnecting to runs in failedSources."""
        assert re.search(r'failedSources\s*\.has\s*\(', js_text), \
            "loadRuns must check failedSources.has(run.id) before reconnecting via openLiveLog"

    def test_loadRuns_syncs_status_icon(self, js_text):
        """loadRuns must sync status icon for non-live completed cards."""
        assert re.search(r"getElementById\s*\(\s*[`'\"]status-icon-\$\{run\.id\}", js_text), \
            "loadRuns must update status icon via getElementById('status-icon-${run.id}')"


# ---------------------------------------------------------------------------
# Task 4: openLiveLog — SSE subscription
# ---------------------------------------------------------------------------

class TestOpenLiveLog:
    def test_openLiveLog_exists(self, js_text):
        """openLiveLog function must be defined."""
        assert re.search(r'function\s+openLiveLog\s*\(', js_text), \
            "openLiveLog function must be defined"

    def test_openLiveLog_gets_pre_by_logout_id(self, js_text):
        """openLiveLog must get pre element by 'logout-${runId}'."""
        assert re.search(r'getElementById\s*\(\s*[`\'"]\s*logout-\$\{runId\}', js_text), \
            "openLiveLog must get pre via getElementById('logout-${runId}')"

    def test_openLiveLog_gets_cursor_by_id(self, js_text):
        """openLiveLog must get cursor element by 'cursor-${runId}'."""
        assert re.search(r'getElementById\s*\(\s*[`\'"]\s*cursor-\$\{runId\}', js_text), \
            "openLiveLog must get cursor via getElementById('cursor-${runId}')"

    def test_openLiveLog_returns_if_no_pre(self, js_text):
        """openLiveLog must return early if pre element not found."""
        assert re.search(r'if\s*\(\s*!pre\s*\)\s*return', js_text), \
            "openLiveLog must guard with 'if (!pre) return'"

    def test_openLiveLog_creates_eventsource(self, js_text):
        """openLiveLog must create an EventSource."""
        assert 'new EventSource(' in js_text, \
            "openLiveLog must create a new EventSource"

    def test_openLiveLog_eventsource_url(self, js_text):
        """openLiveLog EventSource URL must be /api/runs/${runId}/stream."""
        assert re.search(r'EventSource\s*\(\s*`[^`]*api/runs/\$\{runId\}/stream', js_text), \
            "openLiveLog must use /api/runs/${runId}/stream as SSE URL"

    def test_openLiveLog_stores_in_liveSources(self, js_text):
        """openLiveLog must store the EventSource in liveSources[runId]."""
        assert re.search(r'liveSources\s*\[\s*runId\s*\]\s*=\s*src', js_text), \
            "openLiveLog must store EventSource as liveSources[runId] = src"

    def test_openLiveLog_onmessage_parses_json(self, js_text):
        """openLiveLog onmessage must parse e.data as JSON."""
        assert re.search(r'JSON\.parse\s*\(\s*e\.data\s*\)', js_text), \
            "openLiveLog onmessage must use JSON.parse(e.data)"

    def test_openLiveLog_onmessage_guards_chunk(self, js_text):
        """openLiveLog onmessage must return early if no chunk."""
        assert re.search(r'if\s*\(\s*!d\.chunk\s*\)\s*return', js_text), \
            "openLiveLog onmessage must guard with 'if (!d.chunk) return'"

    def test_openLiveLog_onmessage_scroll_threshold(self, js_text):
        """openLiveLog onmessage scroll-bottom check must use threshold of 4."""
        assert re.search(r'clientHeight\s*\+\s*4', js_text), \
            "openLiveLog scroll check must use clientHeight + 4 threshold"

    def test_openLiveLog_onmessage_inserts_text_node(self, js_text):
        """openLiveLog onmessage must insert a text node via createTextNode."""
        assert 'document.createTextNode(d.chunk)' in js_text, \
            "openLiveLog must create text node with document.createTextNode(d.chunk)"

    def test_openLiveLog_onmessage_inserts_before_cursor(self, js_text):
        """openLiveLog onmessage must insert text node before cursor using insertBefore."""
        assert re.search(
            r'insertBefore\s*\(\s*document\.createTextNode\s*\(\s*d\.chunk\s*\)\s*,\s*cursor\s*\)',
            js_text
        ), \
            "openLiveLog must call pre.insertBefore(document.createTextNode(d.chunk), cursor)"

    def test_openLiveLog_listens_for_done_event(self, js_text):
        """openLiveLog must use addEventListener('done', ...) for completion."""
        assert re.search(r"addEventListener\s*\(\s*['\"]done['\"]", js_text), \
            "openLiveLog must listen for 'done' SSE event via addEventListener"

    def test_openLiveLog_done_closes_source(self, js_text):
        """openLiveLog done/error handler must close the EventSource."""
        assert re.search(r'src\.close\s*\(\s*\)', js_text), \
            "openLiveLog done/error handler must call src.close()"

    def test_openLiveLog_done_deletes_from_liveSources(self, js_text):
        """openLiveLog done handler must delete runId from liveSources."""
        assert re.search(r'delete\s+liveSources\s*\[\s*runId\s*\]', js_text), \
            "openLiveLog must delete liveSources[runId] on done/error"

    def test_openLiveLog_done_calls_finalizeRunCard(self, js_text):
        """openLiveLog done handler must call finalizeRunCard."""
        assert re.search(r'finalizeRunCard\s*\(\s*runId', js_text), \
            "openLiveLog done handler must call finalizeRunCard(runId, ...)"

    def test_openLiveLog_done_uses_snake_case_timestamps(self, js_text):
        """openLiveLog done handler must use snake_case payload.started_at and payload.ended_at."""
        assert 'payload.started_at' in js_text and 'payload.ended_at' in js_text, \
            "openLiveLog done handler must use snake_case payload.started_at and payload.ended_at"

    def test_openLiveLog_done_uses_status_or_failed(self, js_text):
        """openLiveLog done handler must use payload.status || 'failed' as fallback."""
        assert re.search(r"payload\.status\s*\|\|\s*['\"]failed['\"]", js_text), \
            "openLiveLog done handler must use payload.status || 'failed'"

    def test_openLiveLog_onerror_adds_to_failedSources(self, js_text):
        """openLiveLog onerror must add runId to failedSources."""
        assert re.search(r'failedSources\s*\.add\s*\(\s*runId\s*\)', js_text), \
            "openLiveLog onerror must call failedSources.add(runId)"

    def test_openLiveLog_onerror_calls_finalizeRunCard_failed(self, js_text):
        """openLiveLog onerror must call finalizeRunCard(runId, 'failed')."""
        assert re.search(r"finalizeRunCard\s*\(\s*runId\s*,\s*['\"]failed['\"]", js_text), \
            "openLiveLog onerror must call finalizeRunCard(runId, 'failed')"


# ---------------------------------------------------------------------------
# Task 4: finalizeRunCard — card state transition
# ---------------------------------------------------------------------------

class TestFinalizeRunCard:
    def test_finalizeRunCard_exists(self, js_text):
        """finalizeRunCard function must be defined."""
        assert re.search(r'function\s+finalizeRunCard\s*\(', js_text), \
            "finalizeRunCard function must be defined"

    def test_finalizeRunCard_uses_optional_chaining(self, js_text):
        """finalizeRunCard must use optional chaining (?.) for null safety."""
        # Verify ?. appears in the finalizeRunCard function body
        match = re.search(r'function\s+finalizeRunCard\s*\(', js_text)
        assert match, "finalizeRunCard must be defined"
        fn_body = js_text[match.start():match.start() + 800]
        assert '?.' in fn_body, \
            "finalizeRunCard must use optional chaining (?.) for double-call safety"

    def test_finalizeRunCard_removes_live_class(self, js_text):
        """finalizeRunCard must remove 'live' class from run-${runId} card."""
        assert re.search(r"getElementById\s*\(`run-\$\{runId\}`\)", js_text) or \
               re.search(r"getElementById\s*\(`run-\$\{runId\}`\)\?\.classList\.remove", js_text) or \
               re.search(r"classList\.remove\s*\(\s*['\"]live['\"]", js_text), \
            "finalizeRunCard must remove 'live' class from run-${runId} card"

    def test_finalizeRunCard_updates_status_icon_id(self, js_text):
        """finalizeRunCard must update status icon by id 'status-icon-${runId}'."""
        assert re.search(r'getElementById\s*\(\s*`status-icon-\$\{runId\}`', js_text), \
            "finalizeRunCard must get status icon via getElementById('status-icon-${runId}')"

    def test_finalizeRunCard_ternary_on_success(self, js_text):
        """finalizeRunCard must ternary on status === 'success' for icon/color."""
        assert re.search(r"status\s*===\s*['\"]success['\"]\s*\?", js_text), \
            "finalizeRunCard must ternary on status === 'success' for icon/color"

    def test_finalizeRunCard_removes_live_badge(self, js_text):
        """finalizeRunCard must remove the live badge element."""
        assert re.search(r'getElementById\s*\(\s*`live-badge-\$\{runId\}`', js_text), \
            "finalizeRunCard must get live-badge element by id live-badge-${runId}"

    def test_finalizeRunCard_changes_label_to_stdout_stderr(self, js_text):
        """finalizeRunCard must change log label to 'stdout + stderr'."""
        assert 'stdout + stderr' in js_text, \
            "finalizeRunCard must set log label to 'stdout + stderr'"

    def test_finalizeRunCard_gets_log_label_by_id(self, js_text):
        """finalizeRunCard must get log label by id 'log-label-${runId}'."""
        assert re.search(r'getElementById\s*\(\s*`log-label-\$\{runId\}`', js_text), \
            "finalizeRunCard must get log label via getElementById('log-label-${runId}')"

    def test_finalizeRunCard_removes_cursor(self, js_text):
        """finalizeRunCard must remove the cursor element."""
        assert re.search(r'getElementById\s*\(\s*`cursor-\$\{runId\}`', js_text), \
            "finalizeRunCard must get cursor element via getElementById('cursor-${runId}')"

    def test_finalizeRunCard_adds_run_failed_class(self, js_text):
        """finalizeRunCard must add run-failed class for non-success."""
        assert re.search(r"classList\.add\s*\(\s*['\"]run-failed['\"]", js_text), \
            "finalizeRunCard must call classList.add('run-failed') for non-success"

    def test_finalizeRunCard_updates_elapsed_time(self, js_text):
        """finalizeRunCard must update run-time-${runId} with timeAgo and durationMs."""
        assert re.search(r'getElementById\s*\(\s*`run-time-\$\{runId\}`', js_text), \
            "finalizeRunCard must update run-time-${runId} element"

    def test_finalizeRunCard_uses_timeAgo_in_time(self, js_text):
        """finalizeRunCard must use timeAgo(startedAt) for elapsed time display."""
        assert re.search(r'timeAgo\s*\(\s*startedAt\s*\)', js_text), \
            "finalizeRunCard must use timeAgo(startedAt) in time display"

    def test_finalizeRunCard_uses_durationMs_in_time(self, js_text):
        """finalizeRunCard must use durationMs(new Date(startedAt), ...) for duration."""
        assert re.search(r'durationMs\s*\(\s*new\s+Date\s*\(\s*startedAt\s*\)', js_text), \
            "finalizeRunCard must use durationMs(new Date(startedAt), ...) in time display"

    def test_finalizeRunCard_guards_time_update(self, js_text):
        """finalizeRunCard must only update time if startedAt and endedAt are present."""
        assert re.search(r'if\s*\(\s*startedAt\s*&&\s*endedAt\s*\)', js_text), \
            "finalizeRunCard must guard time update with 'if (startedAt && endedAt)'"
