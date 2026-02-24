<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { Terminal } from '@xterm/xterm';
  import { FitAddon } from '@xterm/addon-fit';
  import { WebLinksAddon } from '@xterm/addon-web-links';
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
  import { StopLogStream, StopPodLogStream } from '../../wailsjs/go/main/App';
  import { activeLogContainerId, activeContainer, appendLogBatch, type LogMessage } from '../stores/containers';
  import '@xterm/xterm/css/xterm.css';

  // ─── State ──────────────────────────────────────────────────────────────────

  let terminalEl: HTMLDivElement;
  let term: Terminal;
  let fitAddon: FitAddon;
  let resizeObserver: ResizeObserver;

  $: containerId = $activeLogContainerId;
  $: containerName = $activeContainer?.name ?? '';

  // ─── Terminal setup ─────────────────────────────────────────────────────────

  function initTerminal() {
    term = new Terminal({
      theme: {
        background:    '#0D1117',
        foreground:    '#CDD9E5',
        cursor:        '#00D4FF',
        cursorAccent:  '#0D1117',
        black:         '#1C2128',
        red:           '#FF7B72',
        green:         '#3FB950',
        yellow:        '#D29922',
        blue:          '#58A6FF',
        magenta:       '#BC8CFF',
        cyan:          '#39C5CF',
        white:         '#CDD9E5',
        brightBlack:   '#484F58',
        brightRed:     '#FF7B72',
        brightGreen:   '#3FB950',
        brightYellow:  '#E3B341',
        brightBlue:    '#79C0FF',
        brightMagenta: '#D2A8FF',
        brightCyan:    '#56D364',
        brightWhite:   '#FFFFFF',
        selectionBackground: '#264F78',
      },
      fontFamily:  '"JetBrains Mono", "Cascadia Code", "Fira Code", monospace',
      fontSize:    12,
      lineHeight:  1.5,
      scrollback:  5000,
      convertEol:  true,
      cursorBlink: false,
    });

    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.loadAddon(new WebLinksAddon());
    term.open(terminalEl);
    fitAddon.fit();

    // Resize terminal when the panel is resized
    resizeObserver = new ResizeObserver(() => {
      try { fitAddon.fit(); } catch {}
    });
    resizeObserver.observe(terminalEl);
  }

  // ─── Log event handling ─────────────────────────────────────────────────────

  function handleLogBatch(batch: LogMessage[]) {
    if (!term) return;

    // Filter only messages for the active container
    const relevant = batch.filter(m => m.id === containerId);
    if (relevant.length === 0) return;

    appendLogBatch(relevant);

    for (const msg of relevant) {
      term.writeln(msg.line);
    }
  }

  // ─── Close panel ────────────────────────────────────────────────────────────

  function close() {
    if (containerId) {
      // K8s pod streams use "namespace/podName" as ID
      if (containerId.includes('/')) {
        const [namespace, podName] = containerId.split('/');
        StopPodLogStream(namespace, podName);
      } else {
        StopLogStream(containerId);
      }
    }
    activeLogContainerId.set(null);
  }

  function clearTerminal() {
    term?.clear();
  }

  function scrollToBottom() {
    term?.scrollToBottom();
  }

  // ─── Lifecycle ──────────────────────────────────────────────────────────────

  onMount(() => {
    initTerminal();
    term.writeln('\x1b[36m─── Log stream connected ───────────────────────────────\x1b[0m');
    EventsOn('log:batch', handleLogBatch);
  });

  onDestroy(() => {
    EventsOff('log:batch');
    resizeObserver?.disconnect();
    term?.dispose();
  });
</script>

<!-- ─── Panel ──────────────────────────────────────────────────────────────── -->
<div class="log-panel">

  <!-- Header -->
  <div class="log-header">
    <div class="log-title">
      <span class="log-icon">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/>
        </svg>
      </span>
      <span class="container-name">{containerName}</span>
      <span class="live-badge">LIVE</span>
    </div>

    <div class="log-actions">
      <button class="icon-btn" on:click={scrollToBottom} title="Scroll to bottom">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="6 9 12 15 18 9"/>
        </svg>
      </button>
      <button class="icon-btn" on:click={clearTerminal} title="Clear">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>
        </svg>
      </button>
      <button class="icon-btn close-btn" on:click={close} title="Close">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
        </svg>
      </button>
    </div>
  </div>

  <!-- Terminal -->
  <div class="terminal-wrap" bind:this={terminalEl} />

</div>

<style>
  .log-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    background: #0D1117;
    border-left: 1px solid var(--border);
  }

  /* ── Header ── */
  .log-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 16px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
    background: #0D1117;
  }

  .log-title {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .log-icon {
    color: var(--cyan);
    display: flex;
  }

  .container-name {
    font-family: var(--font-mono);
    font-size: 12px;
    font-weight: 600;
    color: var(--text);
  }

  .live-badge {
    font-family: var(--font-mono);
    font-size: 9px;
    font-weight: 700;
    letter-spacing: 0.1em;
    color: var(--green);
    background: color-mix(in srgb, var(--green) 15%, transparent);
    border: 1px solid color-mix(in srgb, var(--green) 30%, transparent);
    padding: 2px 6px;
    border-radius: 3px;
    animation: livePulse 2s ease-in-out infinite;
  }

  @keyframes livePulse {
    0%, 100% { opacity: 1; }
    50%       { opacity: 0.5; }
  }

  .log-actions {
    display: flex;
    gap: 4px;
  }

  .icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    border-radius: 5px;
    border: 1px solid transparent;
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
    transition: all 0.15s;
  }

  .icon-btn:hover {
    border-color: var(--border);
    color: var(--text);
    background: var(--surface-raised);
  }

  .icon-btn.close-btn:hover {
    border-color: var(--red);
    color: var(--red);
    background: color-mix(in srgb, var(--red) 10%, transparent);
  }

  /* ── Terminal ── */
  .terminal-wrap {
    flex: 1;
    overflow: hidden;
    padding: 8px;
  }

  /* Override xterm defaults to fill the container */
  .terminal-wrap :global(.xterm) {
    height: 100%;
  }

  .terminal-wrap :global(.xterm-viewport) {
    background: transparent !important;
  }
</style>