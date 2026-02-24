<script lang="ts">
  import { onMount } from 'svelte';
  import { ListPods, ListNamespaces, StartPodLogStream } from '../../wailsjs/go/main/App';
  import { activeLogContainerId } from '../stores/containers';
  import StatusBadge from './StatusBadge.svelte';

  // ─── Types ──────────────────────────────────────────────────────────────────

  interface PodInfo {
    name: string;
    namespace: string;
    status: string;
    ready: boolean;
    restarts: number;
    nodeName: string;
    age: number;
    image: string;
  }

  // ─── State ──────────────────────────────────────────────────────────────────

  let pods: PodInfo[] = [];
  let namespaces: string[] = [];
  let selectedNamespace: string = '';
  let loading = false;
  let error = '';

  // ─── Load data ──────────────────────────────────────────────────────────────

  async function loadNamespaces() {
    try {
      namespaces = await ListNamespaces();
      if (namespaces.length > 0 && !selectedNamespace) {
        selectedNamespace = namespaces.includes('default') ? 'default' : namespaces[0];
        await loadPods();
      }
    } catch (err) {
      error = 'Failed to connect to Kubernetes cluster';
    }
  }

  async function loadPods() {
    loading = true;
    error = '';
    try {
      pods = await ListPods(selectedNamespace);
    } catch (err) {
      error = `Failed to list pods: ${err}`;
    } finally {
      loading = false;
    }
  }

  async function openLogs(pod: PodInfo) {
    // Use namespace/podName as the ID so the LogViewer can identify the source
    const streamId = `${pod.namespace}/${pod.name}`;
    activeLogContainerId.set(streamId);
    await StartPodLogStream(pod.namespace, pod.name, '');
  }

  // ─── Helpers ────────────────────────────────────────────────────────────────

  function formatAge(unixTs: number): string {
    if (!unixTs) return '—';
    const seconds = Math.floor(Date.now() / 1000) - unixTs;
    if (seconds < 60)   return `${seconds}s`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
    if (seconds < 86400)return `${Math.floor(seconds / 3600)}h`;
    return `${Math.floor(seconds / 86400)}d`;
  }

  // ─── Lifecycle ──────────────────────────────────────────────────────────────

  onMount(loadNamespaces);
</script>

<!-- ─── Panel ──────────────────────────────────────────────────────────────── -->
<div class="panel">

  <!-- Header -->
  <div class="panel-header">
    <span class="panel-title">Pods</span>

    <!-- Namespace selector -->
    <select
      class="ns-select"
      bind:value={selectedNamespace}
      on:change={loadPods}
      disabled={namespaces.length === 0}
    >
      <option value="">All namespaces</option>
      {#each namespaces as ns}
        <option value={ns}>{ns}</option>
      {/each}
    </select>

    <span class="count">{pods.length} pods</span>

    <button class="refresh-btn" on:click={loadPods} disabled={loading}>
      <svg class:spinning={loading} width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M21 2v6h-6M3 12a9 9 0 0 1 15-6.7L21 8M3 22v-6h6M21 12a9 9 0 0 1-15 6.7L3 16"/>
      </svg>
      Refresh
    </button>
  </div>

  <!-- Error state -->
  {#if error}
    <div class="error-state">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--red)" stroke-width="2">
        <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>
      </svg>
      {error}
    </div>

  <!-- Loading -->
  {:else if loading && pods.length === 0}
    <div class="empty-state">
      <div class="spinner" />
      <span>Connecting to cluster...</span>
    </div>

  <!-- Empty -->
  {:else if pods.length === 0}
    <div class="empty-state">
      <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="var(--muted)" stroke-width="1.5">
        <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/>
      </svg>
      <span>No pods found in this namespace</span>
    </div>

  <!-- Table -->
  {:else}
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Namespace</th>
            <th>Status</th>
            <th>Ready</th>
            <th>Restarts</th>
            <th>Node</th>
            <th>Age</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each pods as pod (pod.name + pod.namespace)}
            <tr>
              <td class="cell-name">
                <span class="pod-name">{pod.name}</span>
                <span class="pod-image">{pod.image}</span>
              </td>
              <td>
                <span class="ns-tag">{pod.namespace}</span>
              </td>
              <td>
                <StatusBadge state={pod.status} size="sm" />
              </td>
              <td>
                <span class="ready-badge" class:ready={pod.ready} class:not-ready={!pod.ready}>
                  {pod.ready ? '✓ Ready' : '✗ Not ready'}
                </span>
              </td>
              <td>
                <span class="restarts" class:high={pod.restarts > 5}>
                  {pod.restarts}
                </span>
              </td>
              <td>
                <span class="node-name">{pod.nodeName || '—'}</span>
              </td>
              <td>
                <span class="age">{formatAge(pod.age)}</span>
              </td>
              <td>
                <button
                  class="action-btn logs"
                  on:click={() => openLogs(pod)}
                  title="View logs"
                >
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
                    <polyline points="14 2 14 8 20 8"/>
                    <line x1="8" y1="13" x2="16" y2="13"/>
                    <line x1="8" y1="17" x2="16" y2="17"/>
                  </svg>
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
    flex-wrap: wrap;
  }

  .panel-title {
    font-family: var(--font-mono);
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--text-muted);
  }

  .ns-select {
    padding: 4px 10px;
    background: var(--surface-raised);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text);
    font-size: 12px;
    font-family: var(--font-mono);
    cursor: pointer;
    outline: none;
  }

  .ns-select:focus {
    border-color: var(--cyan);
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

  .refresh-btn:hover { border-color: var(--cyan); color: var(--cyan); }
  .refresh-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .spinning { animation: spin 1s linear infinite; }

  @keyframes spin { to { transform: rotate(360deg); } }

  .empty-state, .error-state {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 12px;
    font-size: 13px;
  }

  .empty-state { color: var(--text-muted); }
  .error-state { color: var(--red); }

  .spinner {
    width: 24px;
    height: 24px;
    border: 2px solid var(--border);
    border-top-color: var(--cyan);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

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

  tbody tr:hover { background: var(--surface-raised); }

  td {
    padding: 10px 16px;
    vertical-align: middle;
  }

  .cell-name { min-width: 200px; }

  .pod-name {
    display: block;
    font-size: 12px;
    font-weight: 500;
    color: var(--text);
    font-family: var(--font-mono);
  }

  .pod-image {
    display: block;
    font-size: 10px;
    color: var(--text-muted);
    margin-top: 2px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 240px;
  }

  .ns-tag {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-dim);
    background: var(--surface-raised);
    padding: 2px 7px;
    border-radius: 4px;
    border: 1px solid var(--border);
  }

  .ready-badge {
    font-family: var(--font-mono);
    font-size: 11px;
  }

  .ready-badge.ready    { color: var(--green); }
  .ready-badge.not-ready{ color: var(--red); }

  .restarts {
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-dim);
  }

  .restarts.high { color: var(--amber); }

  .node-name {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-muted);
  }

  .age {
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-dim);
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
    color: var(--text-muted);
    cursor: pointer;
    transition: all 0.15s;
  }

  .action-btn:hover.logs {
    border-color: var(--cyan);
    color: var(--cyan);
    background: color-mix(in srgb, var(--cyan) 10%, transparent);
  }
</style>