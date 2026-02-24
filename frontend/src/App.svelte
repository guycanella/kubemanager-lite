<script lang="ts">
  import { onMount } from 'svelte';
  import { DockerStatus, K8sStatus } from '../wailsjs/go/main/App';
  import { WindowToggleMaximise } from '../wailsjs/runtime/runtime';
  import { activeTab, activeLogContainerId, dockerConnected, k8sConnected } from './stores/containers';
  import ContainerList from './components/ContainerList.svelte';
  import LogViewer from './components/LogViewer.svelte';
  import PodList from './components/PodList.svelte';
  import Toast from './components/Toast.svelte';

  // ─── Connection check on mount ──────────────────────────────────────────────

  onMount(async () => {
    const [docker, k8s] = await Promise.all([DockerStatus(), K8sStatus()]);
    dockerConnected.set(docker);
    k8sConnected.set(k8s);
  });
</script>

<!-- ─── Root layout ─────────────────────────────────────────────────────────── -->
<div class="app">

  <!-- Titlebar (macOS: transparent + draggable) -->
  <div class="titlebar" on:dblclick={WindowToggleMaximise}>
    <div class="titlebar-left">
      <!-- macOS traffic lights space -->
      <div class="traffic-lights-spacer" />
      <span class="app-name">KubeManager Lite</span>
    </div>

    <nav class="tabs" on:dblclick|stopPropagation>
      <button
        class="tab"
        class:active={$activeTab === 'docker'}
        on:click={() => activeTab.set('docker')}
      >
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18M9 21V9"/>
        </svg>
        Docker
        <span class="status-dot" class:connected={$dockerConnected} />
      </button>

      <button
        class="tab"
        class:active={$activeTab === 'kubernetes'}
        on:click={() => activeTab.set('kubernetes')}
      >
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
        </svg>
        Kubernetes
        <span class="status-dot" class:connected={$k8sConnected} />
      </button>
    </nav>

    <div class="titlebar-right" />
  </div>

  <!-- Main content -->
  <div class="content">

    <!-- Docker tab -->
    {#if $activeTab === 'docker'}
      <div class="pane-layout" class:split={$activeLogContainerId !== null}>
        <div class="pane-main">
          <ContainerList />
        </div>
        {#if $activeLogContainerId}
          <div class="pane-logs">
            <LogViewer />
          </div>
        {/if}
      </div>

    <!-- Kubernetes tab -->
    {:else}
      <div class="pane-layout" class:split={$activeLogContainerId !== null}>
        <div class="pane-main">
          <PodList />
        </div>
        {#if $activeLogContainerId}
          <div class="pane-logs">
            <LogViewer />
          </div>
        {/if}
      </div>
    {/if}

  </div>

  <!-- Global toast notifications (connection status + action errors) -->
  <Toast />

</div>

<style>
  /* ─── Design tokens ────────────────────────────────────────────────────────── */
  :global(:root) {
    --bg:            #0D1117;
    --surface:       #161B22;
    --surface-raised:#1C2128;
    --border:        #30363D;
    --text:          #CDD9E5;
    --text-dim:      #8B949E;
    --text-muted:    #484F58;
    --muted:         #484F58;

    --cyan:   #00D4FF;
    --green:  #3FB950;
    --amber:  #D29922;
    --red:    #F85149;
    --blue:   #58A6FF;

    --font-sans: 'SF Pro Text', 'Helvetica Neue', system-ui, sans-serif;
    --font-mono: 'JetBrains Mono', 'Cascadia Code', 'Fira Code', 'Menlo', monospace;
  }

  :global(*) {
    box-sizing: border-box;
    margin: 0;
    padding: 0;
  }

  :global(body) {
    background: var(--bg);
    color: var(--text);
    font-family: var(--font-sans);
    font-size: 13px;
    line-height: 1.5;
    -webkit-font-smoothing: antialiased;
    overflow: hidden;
    user-select: none;
  }

  :global(::-webkit-scrollbar) {
    width: 6px;
    height: 6px;
  }

  :global(::-webkit-scrollbar-track) {
    background: transparent;
  }

  :global(::-webkit-scrollbar-thumb) {
    background: var(--border);
    border-radius: 3px;
  }

  :global(::-webkit-scrollbar-thumb:hover) {
    background: var(--text-muted);
  }

  /* ─── Layout ───────────────────────────────────────────────────────────────── */
  .app {
    display: flex;
    flex-direction: column;
    height: 100vh;
    background: var(--bg);
  }

  /* ─── Titlebar ─────────────────────────────────────────────────────────────── */
  .titlebar {
    display: flex;
    align-items: center;
    height: 52px;
    background: var(--surface);
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
    --wails-draggable: drag;
    padding: 0 16px;
  }

  .titlebar-left {
    display: flex;
    align-items: center;
    gap: 12px;
    min-width: 160px;
  }

  /* Space for macOS traffic lights (72px wide area) */
  .traffic-lights-spacer {
    width: 72px;
    flex-shrink: 0;
  }

  .app-name {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-dim);
    letter-spacing: 0.02em;
    white-space: nowrap;
  }

  .tabs {
    display: flex;
    gap: 4px;
    --wails-draggable: no-drag;
  }

  .tab {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 6px 14px;
    border-radius: 8px;
    border: 1px solid transparent;
    background: transparent;
    color: var(--text-muted);
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s;
    position: relative;
  }

  .tab:hover {
    color: var(--text-dim);
    background: var(--surface-raised);
  }

  .tab.active {
    color: var(--text);
    background: var(--surface-raised);
    border-color: var(--border);
  }

  .status-dot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: var(--muted);
    flex-shrink: 0;
    transition: background 0.3s;
  }

  .status-dot.connected {
    background: var(--green);
    box-shadow: 0 0 4px var(--green);
  }

  .titlebar-right {
    flex: 1;
  }

  /* ─── Content ──────────────────────────────────────────────────────────────── */
  .content {
    flex: 1;
    overflow: hidden;
    background: var(--surface);
  }

  /* Docker split layout: container list + log panel side by side */
  .pane-layout {
    display: flex;
    height: 100%;
  }

  .pane-main {
    flex: 1;
    overflow: hidden;
    min-width: 0;
  }

  .pane-logs {
    width: 45%;
    min-width: 320px;
    max-width: 700px;
    flex-shrink: 0;
    overflow: hidden;
  }
</style>