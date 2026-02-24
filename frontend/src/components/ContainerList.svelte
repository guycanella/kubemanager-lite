<script lang="ts">
  import { onMount } from 'svelte';
  import { EventsOn } from '../../wailsjs/runtime/runtime';
  import {
    ListContainers,
    StartContainer,
    StopContainer,
    RestartContainer,
    StartLogStream,
    StartStatsStream,
    StopStatsStream,
  } from '../../wailsjs/go/main/App';
  import {
    containers,
    containersLoading,
    activeLogContainerId,
    applyStatsUpdate,
    type StatsUpdate,
  } from '../stores/containers';
  import StatusBadge from './StatusBadge.svelte';

  // ─── Load containers ────────────────────────────────────────────────────────

  // Track which container IDs currently have active stats streams
  // so we can stop streams for containers that disappear
  let activeStatStreams = new Set<string>();

  async function loadContainers() {
    containersLoading.set(true);
    try {
      const list = await ListContainers();
      const incoming = list ?? [];

      // Stop stats streams for containers no longer present
      const incomingIds = new Set(incoming.map(c => c.id));
      for (const id of activeStatStreams) {
        if (!incomingIds.has(id)) {
          StopStatsStream(id);
          activeStatStreams.delete(id);
        }
      }

      containers.set(incoming);

      // Start stats streams for new running containers
      for (const c of incoming) {
        if (c.state === 'running' && !activeStatStreams.has(c.id)) {
          StartStatsStream(c.id);
          activeStatStreams.add(c.id);
        }
      }
    } catch (err) {
      console.error('[ContainerList] Failed to load containers:', err);
    } finally {
      containersLoading.set(false);
    }
  }

  // ─── Actions ────────────────────────────────────────────────────────────────

  let actionLoading: Record<string, boolean> = {};

  async function handleAction(id: string, action: 'start' | 'stop' | 'restart') {
    actionLoading = { ...actionLoading, [id]: true };
    try {
      if (action === 'start')   await StartContainer(id);
      if (action === 'stop')    await StopContainer(id);
      if (action === 'restart') await RestartContainer(id);
      await loadContainers();
    } catch (err) {
      console.error(`[ContainerList] Failed to ${action} container ${id}:`, err);
    } finally {
      actionLoading = { ...actionLoading, [id]: false };
    }
  }

  async function openLogs(id: string, name: string) {
    activeLogContainerId.set(id);
    await StartLogStream(id, name);
  }

  // ─── Lifecycle ──────────────────────────────────────────────────────────────

  onMount(() => {
    loadContainers();

    EventsOn('stats:update', (update: StatsUpdate) => {
      applyStatsUpdate(update);
    });

    EventsOn('container:lifecycle', () => {
      loadContainers();
    });

    // Fallback: refresh every 30s to catch any missed events
    const interval = setInterval(loadContainers, 30_000);
    return () => clearInterval(interval);
  });

  // ─── Helpers ────────────────────────────────────────────────────────────────

  function cpuBarWidth(pct: number): string {
    return `${Math.min(pct, 100).toFixed(1)}%`;
  }

  function memPercent(used: number, limit: number): number {
    if (!limit) return 0;
    return Math.min((used / limit) * 100, 100);
  }

  function barColor(pct: number): string {
    if (pct > 80) return 'var(--red)';
    if (pct > 50) return 'var(--amber)';
    return 'var(--cyan)';
  }
</script>

<!-- ─── Header ──────────────────────────────────────────────────────────────── -->
<div class="panel">
  <div class="panel-header">
    <span class="panel-title">Containers</span>
    <span class="count">{$containers.length} running</span>
    <button class="refresh-btn" on:click={loadContainers} disabled={$containersLoading}>
      <svg class:spinning={$containersLoading} width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M21 2v6h-6M3 12a9 9 0 0 1 15-6.7L21 8M3 22v-6h6M21 12a9 9 0 0 1-15 6.7L3 16"/>
      </svg>
      Refresh
    </button>
  </div>

  <!-- ─── Loading ──────────────────────────────────────────────────────────── -->
  {#if $containersLoading && $containers.length === 0}
    <div class="empty-state">
      <div class="spinner" />
      <span>Connecting to Docker...</span>
    </div>

  <!-- ─── Empty ────────────────────────────────────────────────────────────── -->
  {:else if $containers.length === 0}
    <div class="empty-state">
      <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="var(--muted)" stroke-width="1.5">
        <rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18M9 21V9"/>
      </svg>
      <span>No running containers found</span>
    </div>

  <!-- ─── Table ────────────────────────────────────────────────────────────── -->
  {:else}
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Image</th>
            <th>Status</th>
            <th>CPU</th>
            <th>Memory</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each $containers as c (c.id)}
            <tr class:active={$activeLogContainerId === c.id}>

              <!-- Name + ID -->
              <td class="cell-name">
                <button class="name-btn" on:click={() => openLogs(c.id, c.name)}>
                  {c.name}
                </button>
                <span class="short-id">{c.shortId}</span>
              </td>

              <!-- Image -->
              <td class="cell-image">
                <span class="image-tag">{c.image}</span>
              </td>

              <!-- Status -->
              <td>
                <StatusBadge state={c.state} size="sm" />
              </td>

              <!-- CPU bar -->
              <td class="cell-metric">
                <div class="metric-row">
                  <span class="metric-val">{c.cpuPercent.toFixed(1)}%</span>
                  <div class="bar-track">
                    <div
                      class="bar-fill"
                      style="width: {cpuBarWidth(c.cpuPercent)}; background: {barColor(c.cpuPercent)}"
                    />
                  </div>
                </div>
              </td>

              <!-- Memory bar -->
              <td class="cell-metric">
                <div class="metric-row">
                  <span class="metric-val">{c.memoryUsageMB.toFixed(0)} MB</span>
                  <div class="bar-track">
                    <div
                      class="bar-fill"
                      style="width: {memPercent(c.memoryUsageMB, c.memoryLimitMB).toFixed(1)}%; background: {barColor(memPercent(c.memoryUsageMB, c.memoryLimitMB))}"
                    />
                  </div>
                </div>
              </td>

              <!-- Actions -->
              <td class="cell-actions">
                {#if c.state === 'running'}
                  <button
                    class="action-btn stop"
                    disabled={actionLoading[c.id]}
                    on:click={() => handleAction(c.id, 'stop')}
                    title="Stop"
                  >
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><rect x="4" y="4" width="16" height="16" rx="2"/></svg>
                  </button>
                  <button
                    class="action-btn restart"
                    disabled={actionLoading[c.id]}
                    on:click={() => handleAction(c.id, 'restart')}
                    title="Restart"
                  >
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M21 2v6h-6M3 12a9 9 0 0 1 15-6.7L21 8"/></svg>
                  </button>
                {:else}
                  <button
                    class="action-btn start"
                    disabled={actionLoading[c.id]}
                    on:click={() => handleAction(c.id, 'start')}
                    title="Start"
                  >
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><polygon points="5,3 19,12 5,21"/></svg>
                  </button>
                {/if}
                <button
                  class="action-btn logs"
                  on:click={() => openLogs(c.id, c.name)}
                  title="View logs"
                >
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="16" y2="17"/></svg>
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<style>
  .panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
  }

  .panel-header {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 16px 20px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }

  .panel-title {
    font-family: var(--font-mono);
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--text-muted);
  }

  .count {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--cyan);
    margin-right: auto;
  }

  .refresh-btn {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 5px 10px;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-muted);
    font-size: 12px;
    cursor: pointer;
    transition: all 0.15s;
  }

  .refresh-btn:hover {
    border-color: var(--cyan);
    color: var(--cyan);
  }

  .refresh-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .spinning {
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  /* ── Empty / loading state ── */
  .empty-state {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 12px;
    color: var(--text-muted);
    font-size: 13px;
  }

  .spinner {
    width: 24px;
    height: 24px;
    border: 2px solid var(--border);
    border-top-color: var(--cyan);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  /* ── Table ── */
  .table-wrap {
    flex: 1;
    overflow-y: auto;
  }

  table {
    width: 100%;
    border-collapse: collapse;
  }

  thead th {
    padding: 10px 16px;
    text-align: left;
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
    border-bottom: 1px solid var(--border);
    white-space: nowrap;
    position: sticky;
    top: 0;
    background: var(--surface);
    z-index: 1;
  }

  tbody tr {
    border-bottom: 1px solid color-mix(in srgb, var(--border) 50%, transparent);
    transition: background 0.1s;
  }

  tbody tr:hover {
    background: var(--surface-raised);
  }

  tbody tr.active {
    background: color-mix(in srgb, var(--cyan) 6%, transparent);
    border-bottom-color: color-mix(in srgb, var(--cyan) 20%, transparent);
  }

  td {
    padding: 10px 16px;
    vertical-align: middle;
  }

  /* ── Name cell ── */
  .cell-name {
    min-width: 160px;
  }

  .name-btn {
    display: block;
    background: none;
    border: none;
    padding: 0;
    color: var(--text);
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    text-align: left;
    transition: color 0.15s;
  }

  .name-btn:hover {
    color: var(--cyan);
  }

  .short-id {
    display: block;
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-muted);
    margin-top: 2px;
  }

  /* ── Image cell ── */
  .image-tag {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-dim);
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: block;
  }

  /* ── Metric cell ── */
  .cell-metric {
    min-width: 110px;
  }

  .metric-row {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .metric-val {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-dim);
  }

  .bar-track {
    height: 3px;
    background: var(--surface-raised);
    border-radius: 2px;
    overflow: hidden;
    width: 80px;
  }

  .bar-fill {
    height: 100%;
    border-radius: 2px;
    transition: width 0.6s ease, background 0.3s;
  }

  /* ── Action buttons ── */
  .cell-actions {
    white-space: nowrap;
  }

  .action-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: transparent;
    cursor: pointer;
    transition: all 0.15s;
    margin-right: 4px;
    color: var(--text-muted);
  }

  .action-btn:hover.start  { border-color: var(--green); color: var(--green); background: color-mix(in srgb, var(--green) 10%, transparent); }
  .action-btn:hover.stop   { border-color: var(--red);   color: var(--red);   background: color-mix(in srgb, var(--red) 10%, transparent); }
  .action-btn:hover.restart{ border-color: var(--amber); color: var(--amber); background: color-mix(in srgb, var(--amber) 10%, transparent); }
  .action-btn:hover.logs   { border-color: var(--cyan);  color: var(--cyan);  background: color-mix(in srgb, var(--cyan) 10%, transparent); }

  .action-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
</style>