<script lang="ts">
    export let state: string = '';
    export let size: 'sm' | 'md' = 'md';
  
    $: color = {
      running:    'var(--green)',
      paused:     'var(--amber)',
      exited:     'var(--muted)',
      dead:       'var(--red)',
      created:    'var(--cyan)',
      restarting: 'var(--amber)',
    }[state.toLowerCase()] ?? 'var(--muted)';
  </script>
  
  <span class="badge" class:sm={size === 'sm'} style="--dot-color: {color}">
    <span class="dot" />
    {state}
  </span>
  
  <style>
    .badge {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      font-family: var(--font-mono);
      font-size: 12px;
      font-weight: 500;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      color: var(--dot-color);
      padding: 3px 8px;
      border-radius: 4px;
      background: color-mix(in srgb, var(--dot-color) 12%, transparent);
      border: 1px solid color-mix(in srgb, var(--dot-color) 25%, transparent);
      white-space: nowrap;
    }
  
    .badge.sm {
      font-size: 10px;
      padding: 2px 6px;
      gap: 4px;
    }
  
    .dot {
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: var(--dot-color);
      flex-shrink: 0;
    }
  
    /* Pulse animation for running containers only */
    :global(.badge[style*="--green"] .dot) {
      animation: pulse 2.5s ease-in-out infinite;
    }
  
    @keyframes pulse {
      0%, 100% { opacity: 1; }
      50%       { opacity: 0.4; }
    }
  </style>