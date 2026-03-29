// loom web UI

const API = '';
let daemonState = null;

window.addEventListener('DOMContentLoaded', () => {
  loadAll();
  setInterval(loadAll, 3000);
  loadSettings();
  loadChatHistory();
  onTriggerTypeChange();
  onExecutorChange();
  document.getElementById('chat-input').addEventListener('keydown', e => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendChat(); }
  });
  routeFromHash();
  window.addEventListener('hashchange', routeFromHash);
});

async function loadAll() {
  await Promise.all([loadStatus(), loadJobs(), loadRuns()]);
}

// ── Status ────────────────────────────────────────────────────────────────────

async function loadStatus() {
  try {
    const s = await api('GET', '/api/status');
    daemonState = s;
    const badge = document.getElementById('status-badge');
    badge.className = `badge badge-${s.state}`;
    badge.textContent = s.state;
    document.getElementById('header-stats').textContent =
      `${s.activeRuns} running · ${s.queueDepth} queued · ${s.jobCount} jobs · v${s.version}`;
    document.getElementById('btn-pause').textContent = s.state === 'paused' ? 'Resume' : 'Pause';
  } catch {
    const badge = document.getElementById('status-badge');
    badge.className = 'badge badge-stopped';
    badge.textContent = 'offline';
  }
}

async function togglePause() {
  if (!daemonState) return;
  await api('POST', `/api/daemon/${daemonState.state === 'paused' ? 'resume' : 'pause'}`);
  await loadStatus();
}

async function flushQueue() {
  await api('POST', '/api/daemon/flush');
  await loadStatus();
}

async function pruneJobs() {
  const disabled = jobs.filter(j => !j.enabled);
  if (!disabled.length) { alert('No disabled jobs to prune.'); return; }
  if (!confirm(`Delete ${disabled.length} disabled job(s)?`)) return;
  const res = await api('POST', '/api/jobs/prune');
  await loadJobs();
  alert(`✓ Pruned ${res.deleted} job(s).`);
}

// ── Jobs ──────────────────────────────────────────────────────────────────────

let jobs = [];

async function loadJobs() {
  try {
    jobs = await api('GET', '/api/jobs');
    renderJobs();
  } catch {}
}

const executorIcons = { shell: '⌘', 'claude-code': '✦', amplifier: '⚡' };
const executorLabels = { shell: 'shell', 'claude-code': 'claude code', amplifier: 'amplifier' };

function renderJobs() {
  const container = document.getElementById('jobs-list');
  if (!jobs.length) {
    container.innerHTML = '<div class="empty">No jobs yet.<br/>Click "+ Add Job" to create one.</div>';
    return;
  }
  container.innerHTML = jobs.map(renderJobCard).join('');
}

function renderJobCard(job) {
  const exec = job.executor || 'shell';
  const triggerLabel = job.trigger.type === 'watch' && job.watch
    ? `watch: ${job.watch.path}`
    : job.trigger.schedule
      ? `${job.trigger.type}: ${job.trigger.schedule}`
      : job.trigger.type;
  return `
  <div class="job-card ${job.enabled ? '' : 'disabled'}">
    <div class="job-card-header">
      <span class="job-name">${esc(job.name)}</span>
      <div class="job-badges">
        <span class="executor-badge exec-${exec}" title="${exec}">${executorIcons[exec] || '⌘'} ${executorLabels[exec] || exec}</span>
        <span class="trigger-badge trigger-${job.trigger.type}">${job.trigger.type}</span>
        <span class="dot ${job.enabled ? 'dot-enabled' : 'dot-disabled'}"></span>
      </div>
    </div>
    <div class="job-trigger">${esc(triggerLabel)}</div>
    ${job.description ? `<div class="job-description">${esc(job.description)}</div>` : ''}
    ${renderJobSummary(job)}
    <div class="job-actions">
      <button class="btn btn-ghost btn-sm" onclick="triggerJob('${job.id}')">▶ Run now</button>
      <button class="btn btn-ghost btn-sm" onclick="openEditJobModal('${job.id}')">Edit</button>
      <button class="btn btn-ghost btn-sm" onclick="toggleJobEnabled('${job.id}', ${!job.enabled})">${job.enabled ? 'Disable' : 'Enable'}</button>
      <button class="btn btn-ghost btn-sm" style="color:var(--red)" onclick="deleteJob('${job.id}')">Delete</button>
    </div>
  </div>`;
}

function renderJobSummary(job) {
  const exec = job.executor || 'shell';
  if (exec === 'shell') {
    const cmd = (job.shell && job.shell.command) || job.command || '';
    return cmd ? `<div class="job-cmd">${esc(cmd)}</div>` : '';
  }
  if (exec === 'claude-code' && job.claudeCode) {
    const steps = (job.claudeCode.steps || []).length;
    return `<div class="job-cmd">${esc(job.claudeCode.prompt.slice(0, 80))}${job.claudeCode.prompt.length > 80 ? '…' : ''}${steps ? ` <span class="hint-inline">+${steps} step${steps>1?'s':''}</span>` : ''}</div>`;
  }
  if (exec === 'amplifier' && job.amplifier) {
    if (job.amplifier.recipePath) return `<div class="job-cmd">recipe: ${esc(job.amplifier.recipePath)}</div>`;
    if (job.amplifier.prompt) return `<div class="job-cmd">${esc(job.amplifier.prompt.slice(0, 80))}${job.amplifier.prompt.length > 80 ? '…' : ''}</div>`;
  }
  return '';
}

async function triggerJob(id) {
  await api('POST', `/api/jobs/${id}/trigger`);
  setTimeout(loadRuns, 500);
}

async function toggleJobEnabled(id, enable) {
  await api('POST', `/api/jobs/${id}/${enable ? 'enable' : 'disable'}`);
  await loadJobs();
}

async function deleteJob(id) {
  const job = jobs.find(j => j.id === id);
  if (!confirm(`Delete job "${job?.name}"?`)) return;
  await api('DELETE', `/api/jobs/${id}`);
  await loadJobs();
}

// ── Modal ─────────────────────────────────────────────────────────────────────

function openAddJobModal() {
  document.getElementById('modal-title').textContent = 'Add Job';
  document.getElementById('job-form').reset();
  document.getElementById('f-job-id').value = '';
  document.getElementById('f-executor').value = 'shell';
  document.getElementById('f-trigger-type').value = 'once';
  onTriggerTypeChange();
  onExecutorChange();
  document.getElementById('modal-overlay').classList.remove('hidden');
}

function openEditJobModal(id) {
  const job = jobs.find(j => j.id === id);
  if (!job) return;
  document.getElementById('modal-title').textContent = 'Edit Job';
  document.getElementById('f-job-id').value = job.id;
  document.getElementById('f-name').value = job.name;
  document.getElementById('f-description').value = job.description || '';
  document.getElementById('f-cwd').value = job.cwd || '';
  document.getElementById('f-trigger-type').value = job.trigger.type;
  document.getElementById('f-trigger-schedule').value = job.trigger.schedule || '';
  document.getElementById('f-timeout').value = job.timeout || '';
  document.getElementById('f-retries').value = job.maxRetries || 0;

  const exec = job.executor || 'shell';
  document.getElementById('f-executor').value = exec;

  if (exec === 'shell') {
    document.getElementById('f-shell-command').value = (job.shell && job.shell.command) || job.command || '';
  } else if (exec === 'claude-code' && job.claudeCode) {
    document.getElementById('f-claude-prompt').value = job.claudeCode.prompt || '';
    document.getElementById('f-claude-steps').value = (job.claudeCode.steps || []).join('\n');
    document.getElementById('f-claude-model').value = job.claudeCode.model || '';
    document.getElementById('f-claude-max-turns').value = job.claudeCode.maxTurns || '';
    document.getElementById('f-claude-tools').value = (job.claudeCode.allowedTools || []).join(',');
  } else if (exec === 'amplifier' && job.amplifier) {
    document.getElementById('f-amp-prompt').value = job.amplifier.prompt || '';
    document.getElementById('f-amp-steps').value = (job.amplifier.steps || []).join('\n');
    document.getElementById('f-amp-recipe').value = job.amplifier.recipePath || '';
    document.getElementById('f-amp-bundle').value = job.amplifier.bundle || '';
    document.getElementById('f-amp-model').value = job.amplifier.model || '';
  }

  if (job.trigger.type === 'watch' && job.watch) {
    document.getElementById('f-watch-path').value = job.watch.path || '';
    document.getElementById('f-watch-mode').value = job.watch.mode || 'notify';
    document.getElementById('f-watch-poll-interval').value = job.watch.pollInterval || '';
    document.getElementById('f-watch-debounce').value = job.watch.debounce || '';
    document.getElementById('f-watch-recursive').checked = !!job.watch.recursive;
    document.getElementById('f-watch-events').value = (job.watch.events || []).join(',');
  }

  onTriggerTypeChange();
  onExecutorChange();
  document.getElementById('modal-overlay').classList.remove('hidden');
}

function closeModal(e) {
  if (e && e.target !== document.getElementById('modal-overlay')) return;
  document.getElementById('modal-overlay').classList.add('hidden');
}

function onTriggerTypeChange() {
  const type = document.getElementById('f-trigger-type').value;
  const hints = {
    once: 'Leave empty to run right now, or set a delay: "5m", "2h"',
    loop: 'Duration: "30s", "5m", "1h", "24h"',
    cron: 'Cron with seconds: "0 */5 * * * *" = every 5 min',
    watch: 'Fires when the watched path changes',
  };
  document.getElementById('trigger-hint').textContent = hints[type] || '';
  const isWatch = type === 'watch';
  document.getElementById('f-trigger-schedule').closest('.trigger-row').style.display = isWatch ? 'none' : '';
  document.getElementById('trigger-watch').classList.toggle('hidden', !isWatch);
}

function onExecutorChange() {
  const exec = document.getElementById('f-executor').value;
  document.getElementById('exec-shell').classList.toggle('hidden', exec !== 'shell');
  document.getElementById('exec-claude-code').classList.toggle('hidden', exec !== 'claude-code');
  document.getElementById('exec-amplifier').classList.toggle('hidden', exec !== 'amplifier');
}

async function submitJob(e) {
  e.preventDefault();
  const id = document.getElementById('f-job-id').value;
  const exec = document.getElementById('f-executor').value;

  const body = {
    name: document.getElementById('f-name').value.trim(),
    description: document.getElementById('f-description').value.trim(),
    cwd: document.getElementById('f-cwd').value.trim(),
    trigger: {
      type: document.getElementById('f-trigger-type').value,
      schedule: document.getElementById('f-trigger-schedule').value.trim(),
    },
    executor: exec,
    timeout: document.getElementById('f-timeout').value.trim(),
    maxRetries: parseInt(document.getElementById('f-retries').value) || 0,
    enabled: true,
  };

  if (exec === 'shell') {
    body.shell = { command: document.getElementById('f-shell-command').value.trim() };
  } else if (exec === 'claude-code') {
    const stepsRaw = document.getElementById('f-claude-steps').value.trim();
    body.claudeCode = {
      prompt: document.getElementById('f-claude-prompt').value.trim(),
      steps: stepsRaw ? stepsRaw.split('\n').map(s => s.trim()).filter(Boolean) : [],
      model: document.getElementById('f-claude-model').value.trim(),
      maxTurns: parseInt(document.getElementById('f-claude-max-turns').value) || 0,
      allowedTools: document.getElementById('f-claude-tools').value.trim().split(',').map(s => s.trim()).filter(Boolean),
    };
  } else if (exec === 'amplifier') {
    const stepsRaw = document.getElementById('f-amp-steps').value.trim();
    body.amplifier = {
      prompt: document.getElementById('f-amp-prompt').value.trim(),
      steps: stepsRaw ? stepsRaw.split('\n').map(s => s.trim()).filter(Boolean) : [],
      recipePath: document.getElementById('f-amp-recipe').value.trim(),
      bundle: document.getElementById('f-amp-bundle').value.trim(),
      model: document.getElementById('f-amp-model').value.trim(),
    };
  }

  if (body.trigger.type === 'watch') {
    const eventsRaw = document.getElementById('f-watch-events').value.trim();
    body.watch = {
      path: document.getElementById('f-watch-path').value.trim(),
      mode: document.getElementById('f-watch-mode').value,
      pollInterval: document.getElementById('f-watch-poll-interval').value.trim(),
      debounce: document.getElementById('f-watch-debounce').value.trim(),
      recursive: document.getElementById('f-watch-recursive').checked,
      events: eventsRaw ? eventsRaw.split(',').map(s => s.trim()).filter(Boolean) : [],
    };
    body.trigger.schedule = '';
  }

  try {
    if (id) {
      await api('PUT', `/api/jobs/${id}`, body);
    } else {
      await api('POST', '/api/jobs', body);
    }
    document.getElementById('modal-overlay').classList.add('hidden');
    await loadJobs();
  } catch (err) {
    alert('Error: ' + err.message);
  }
}

// ── Runs ──────────────────────────────────────────────────────────────────────

const liveSources   = {};          // runId → EventSource (open SSE connections)
const failedSources = new Set();   // runIds that errored via SSE — never reconnect

async function loadRuns() {
  let runs;
  try {
    runs = await api('GET', '/api/runs?limit=30');
  } catch { return; }

  const list = document.getElementById('runs-list');

  if (!runs.length) {
    if (!list.querySelector('.run-card')) {
      list.innerHTML = '<div class="empty">No activity yet.</div>';
    }
    return;
  }

  // Remove stale empty-state placeholder if runs appeared
  const emptyEl = list.querySelector('.empty');
  if (emptyEl) emptyEl.remove();

  // Build set of current run IDs from API response
  const currentIds = new Set(runs.map(r => r.id));

  // Remove cards no longer in the API response
  list.querySelectorAll('.run-card').forEach(el => {
    const runId = el.id.replace('run-', '');
    if (!currentIds.has(runId)) {
      if (liveSources[runId]) {
        liveSources[runId].close();
        delete liveSources[runId];
      }
      el.remove();
    }
  });

  // Iterate oldest-first so insertAdjacentHTML('afterbegin') leaves newest at top
  [...runs].reverse().forEach(run => {
    const existing = document.getElementById(`run-${run.id}`);

    if (existing) {
      // Update elapsed time in-place
      const timeEl = document.getElementById(`run-time-${run.id}`);
      if (timeEl) {
        if (run.status === 'running') {
          timeEl.textContent = `started ${timeAgo(run.startedAt)}`;
        } else {
          timeEl.textContent = `${timeAgo(run.startedAt)} · ${durationMs(new Date(run.startedAt), new Date(run.endedAt))}`;
        }
      }

      // For non-live completed cards: sync status icon text/color and run-failed class
      if (!liveSources[run.id] && run.status !== 'running') {
        const iconEl = document.getElementById(`status-icon-${run.id}`);
        if (iconEl) {
          const isFailed = run.status !== 'success';
          iconEl.textContent = isFailed ? '✕' : '✓';
          iconEl.style.color = isFailed ? 'var(--red)' : 'var(--green)';
          if (isFailed) {
            existing.classList.add('run-failed');
          } else {
            existing.classList.remove('run-failed');
          }
        }
      }

      // Start SSE if run is still running, no source exists, and not in failedSources
      if (run.status === 'running' && !liveSources[run.id] && !failedSources.has(run.id)) {
        openLiveLog(run.id);
      }
    } else {
      // New card — insert at top
      list.insertAdjacentHTML('afterbegin', renderRunCard(run));
      if (run.status === 'running') {
        openLiveLog(run.id);
      }
    }
  });
}

async function clearActivity() {
  if (!confirm('Clear all activity history?')) return;
  await api('DELETE', '/api/runs');
  document.getElementById('runs-list').innerHTML = '<div class="empty">No activity yet.</div>';
}

function renderRunCard(run) {
  const name = esc(run.jobName || run.jobId);
  const attemptSuffix = run.attempt > 1 ? ` · attempt ${run.attempt}` : '';

  if (run.status === 'running') {
    return `
  <div class="run-card live" id="run-${run.id}">
    <div class="run-header">
      <span class="run-status-icon" id="status-icon-${run.id}" style="color:#2196F3">●</span>
      <span class="run-name">${name}${attemptSuffix}</span>
      <span class="run-time" id="run-time-${run.id}">started ${timeAgo(run.startedAt)}</span>
      <span class="live-badge" id="live-badge-${run.id}">● live</span>
      <a href="#" class="log-toggle" onclick="toggleLog('${run.id}', this);return false">hide ▴</a>
    </div>
    <div class="log-panel" id="log-${run.id}">
      <div class="log-toolbar">
        <span class="log-label" id="log-label-${run.id}">streaming output</span>
        <button class="log-copy-btn" onclick="copyLog('${run.id}')">copy</button>
      </div>
      <pre class="log-output" id="logout-${run.id}"><span id="cursor-${run.id}" class="log-cursor">▌</span></pre>
    </div>
  </div>`;
  }

  const isFailed = run.status !== 'success';
  const icon = run.status === 'success' ? '✓' : '✕';
  const iconColor = run.status === 'success' ? 'var(--green)' : 'var(--red)';
  const failedClass = isFailed ? ' run-failed' : '';

  return `
  <div class="run-card${failedClass}" id="run-${run.id}">
    <div class="run-header">
      <span class="run-status-icon" style="color:${iconColor}">${icon}</span>
      <span class="run-name">${name}${attemptSuffix}</span>
      <span class="run-time">${timeAgo(run.startedAt)} · ${durationMs(new Date(run.startedAt), new Date(run.endedAt))}</span>
      <a href="#" class="log-toggle" onclick="toggleLog('${run.id}', this);return false">logs ▾</a>
    </div>
    <div class="log-panel hidden" id="log-${run.id}">
      <div class="log-toolbar">
        <span class="log-label" id="log-label-${run.id}">output</span>
        <button class="log-copy-btn" onclick="copyLog('${run.id}')">copy</button>
      </div>
      <pre class="log-output" id="logout-${run.id}">${esc(run.output || '')}</pre>
    </div>
  </div>`;
}

function toggleLog(id, link) {
  const panel = document.getElementById(`log-${id}`);
  if (!panel) return;
  const hidden = panel.classList.toggle('hidden');
  link.textContent = hidden ? 'logs ▾' : 'hide ▴';
}

function copyLog(runId) {
  const pre = document.getElementById(`logout-${runId}`);
  if (!pre) return;
  const text = Array.from(pre.childNodes)
    .filter(n => n.id !== `cursor-${runId}`)
    .map(n => n.textContent)
    .join('');
  navigator.clipboard.writeText(text).catch(() => {});
}

function openLiveLog(runId) {
  const pre = document.getElementById(`logout-${runId}`);
  if (!pre) return;
  const cursor = document.getElementById(`cursor-${runId}`);
  const src = new EventSource(`/api/runs/${runId}/stream`);
  liveSources[runId] = src;
  src.onmessage = e => {
    let d;
    try { d = JSON.parse(e.data); } catch { return; }
    if (!d.chunk) return;
    const atBottom = pre.scrollHeight - pre.scrollTop <= pre.clientHeight + 4;
    pre.insertBefore(document.createTextNode(d.chunk), cursor);
    if (atBottom) pre.scrollTop = pre.scrollHeight;
  };
  src.addEventListener('done', e => {
    src.close();
    delete liveSources[runId];
    let payload;
    try { payload = JSON.parse(e.data); } catch { payload = {}; }
    finalizeRunCard(runId, payload.status || 'failed', payload.started_at, payload.ended_at);
  });
  src.onerror = () => {
    src.close();
    delete liveSources[runId];
    failedSources.add(runId);
    finalizeRunCard(runId, 'failed');
  };
}

function finalizeRunCard(runId, status, startedAt, endedAt) {
  const cardEl = document.getElementById(`run-${runId}`);
  cardEl?.classList.remove('live');
  const iconEl = document.getElementById(`status-icon-${runId}`);
  if (iconEl) {
    iconEl.textContent = status === 'success' ? '✓' : '✗';
    iconEl.style.color = status === 'success' ? 'var(--green)' : 'var(--red)';
  }
  document.getElementById(`live-badge-${runId}`)?.remove();
  const labelEl = document.getElementById(`log-label-${runId}`);
  if (labelEl) labelEl.textContent = 'stdout + stderr';
  document.getElementById(`cursor-${runId}`)?.remove();
  if (status !== 'success') {
    cardEl?.classList.add('run-failed');
  }
  if (startedAt && endedAt) {
    const timeEl = document.getElementById(`run-time-${runId}`);
    if (timeEl) timeEl.textContent = `${timeAgo(startedAt)} · ${durationMs(new Date(startedAt), new Date(endedAt))}`;
  }
}

// ── Chat ──────────────────────────────────────────────────────────────────────

async function loadChatHistory() {
  try {
    const msgs = await api('GET', '/api/chat/history');
    const container = document.getElementById('chat-messages');
    container.innerHTML = '';
    msgs.forEach(m => appendChatBubble(m.role, m.content));
  } catch {}
}

async function clearChat() {
  if (!confirm('Clear all chat history?')) return;
  await api('DELETE', '/api/chat/history');
  document.getElementById('chat-messages').innerHTML = '';
}

async function sendChat() {
  const input = document.getElementById('chat-input');
  const message = input.value.trim();
  if (!message) return;
  input.value = '';
  appendChatBubble('user', message);
  const thinking = appendChatBubble('assistant', '…');
  try {
    const res = await api('POST', '/api/chat', { message });
    thinking.remove();
    const bubble = appendChatBubble('assistant', res.text);
    if (res.actions?.length) {
      const actDiv = document.createElement('div');
      actDiv.className = 'actions';
      actDiv.textContent = '✓ ' + res.actions.join('\n✓ ');
      bubble.appendChild(actDiv);
    }
    await loadJobs();
  } catch (err) {
    thinking.remove();
    if (err.message === 'no_api_key') {
      appendChatBubble('assistant', 'No API key configured. Go to the Settings tab to add your Anthropic or OpenAI key.');
    } else {
      appendChatBubble('assistant', `Error: ${err.message}`);
    }
  }
}

function appendChatBubble(role, text) {
  const container = document.getElementById('chat-messages');
  const div = document.createElement('div');
  div.className = `chat-bubble ${role}`;
  if (role === 'assistant' && typeof marked !== 'undefined') {
    div.innerHTML = marked.parse(text);
  } else {
    div.textContent = text;
  }
  container.appendChild(div);
  container.scrollTop = container.scrollHeight;
  return div;
}

// ── Settings ──────────────────────────────────────────────────────────────────

async function loadSettings() {
  try {
    const s = await api('GET', '/api/settings');
    document.getElementById('s-provider').value = s.aiProvider || 'anthropic';
    onProviderChange();
    // Don't pre-fill key fields — just show placeholder if set
    if (s.anthropicKeySet) document.getElementById('s-anthropic-key').placeholder = '(key saved — enter new to replace)';
    if (s.anthropicModel) document.getElementById('s-anthropic-model').value = s.anthropicModel;
    if (s.openAIKeySet) document.getElementById('s-openai-key').placeholder = '(key saved — enter new to replace)';
    if (s.openAIModel) document.getElementById('s-openai-model').value = s.openAIModel;
    updateChatOnboarding(s);
  } catch {}
}

function onProviderChange() {
  const p = document.getElementById('s-provider').value;
  document.getElementById('s-anthropic-fields').classList.toggle('hidden', p !== 'anthropic');
  document.getElementById('s-openai-fields').classList.toggle('hidden', p !== 'openai');
}

async function saveSettings() {
  const body = {
    aiProvider: document.getElementById('s-provider').value,
    anthropicKey: document.getElementById('s-anthropic-key').value.trim(),
    anthropicModel: document.getElementById('s-anthropic-model').value,
    openAIKey: document.getElementById('s-openai-key').value.trim(),
    openAIModel: document.getElementById('s-openai-model').value,
  };
  try {
    const s = await api('PUT', '/api/settings', body);
    document.getElementById('s-anthropic-key').value = '';
    document.getElementById('s-openai-key').value = '';
    if (s.anthropicKeySet) document.getElementById('s-anthropic-key').placeholder = '(key saved — enter new to replace)';
    if (s.openAIKeySet) document.getElementById('s-openai-key').placeholder = '(key saved — enter new to replace)';
    const saved = document.getElementById('settings-saved');
    saved.style.display = 'block';
    setTimeout(() => { saved.style.display = 'none'; }, 3000);
    updateChatOnboarding(s);
  } catch (err) {
    alert('Error saving settings: ' + err.message);
  }
}

function updateChatOnboarding(settings) {
  const configured = !!settings.aiConfigured;
  const chatBanner = document.getElementById('chat-onboarding');
  if (chatBanner) chatBanner.classList.toggle('hidden', configured);
  const nudge = document.getElementById('ai-setup-nudge');
  if (nudge) nudge.classList.toggle('hidden', configured);
}

async function testConnection() {
  const btn = event.target;
  const result = document.getElementById('settings-test-result');
  btn.disabled = true;
  btn.textContent = 'Testing…';
  result.style.display = 'none';
  try {
    const res = await api('POST', '/api/settings/test', {});
    result.style.display = 'block';
    result.style.color = res.ok ? 'var(--green)' : 'var(--red)';
    result.textContent = (res.ok ? '✓ ' : '✗ ') + res.message;
  } catch (err) {
    result.style.display = 'block';
    result.style.color = 'var(--red)';
    result.textContent = '✗ ' + err.message;
  } finally {
    btn.disabled = false;
    btn.textContent = 'Test Connection';
  }
}

// ── Tabs / routing ────────────────────────────────────────────────────────────

const TAB_NAMES = ['runs', 'chat', 'settings'];

function switchTab(name, btn) {
  document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
  document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
  btn.classList.add('active');
  document.getElementById(`tab-${name}`).classList.add('active');
  history.replaceState(null, '', `#${name}`);
}

function switchTabByName(name) {
  const btn = document.querySelector(`.tab[onclick*="'${name}'"]`);
  if (btn) switchTab(name, btn);
}

function routeFromHash() {
  const hash = location.hash.replace('#', '') || 'runs';
  const name = TAB_NAMES.includes(hash) ? hash : 'runs';
  switchTabByName(name);
}

// ── Utils ─────────────────────────────────────────────────────────────────────

async function api(method, path, body) {
  const opts = { method, headers: { 'Content-Type': 'application/json' } };
  if (body) opts.body = JSON.stringify(body);
  const res = await fetch(API + path, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  return data;
}

function esc(str) {
  return String(str).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function timeAgo(isoStr) {
  const s = Math.floor((Date.now() - new Date(isoStr)) / 1000);
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s/60)}m ago`;
  if (s < 86400) return `${Math.floor(s/3600)}h ago`;
  return `${Math.floor(s/86400)}d ago`;
}

function durationMs(start, end) {
  const ms = end - start;
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms/1000).toFixed(1)}s`;
  return `${Math.floor(ms/60000)}m ${Math.floor((ms%60000)/1000)}s`;
}
