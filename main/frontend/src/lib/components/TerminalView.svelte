<script>
  import { onMount } from "svelte";
  import { Clipboard } from "@wailsio/runtime";
  import { FitAddon } from "@xterm/addon-fit";
  import { Terminal } from "@xterm/xterm";

  export let theme = {
    background: "#0b0b0c",
    foreground: "#f5f7ff",
    cursor: "#8bd3ff",
  };

  export let onReady = () => {};
  export let onInput = () => {};

  let terminalElement;
  let term;
  let fitAddon;
  let pendingOutput = [];
  let flushScheduled = false;
  let flushFrame = 0;
  let suppressTerminalInput = false;
  let contextMenuOpen = false;
  let contextMenuX = 0;
  let contextMenuY = 0;
  let contextMenuCanCopy = false;

  const contextMenuWidth = 228;
  const contextMenuHeight = 104;

  function focusTerminal() {
    term?.focus();
  }

  function closeContextMenu() {
    contextMenuOpen = false;
  }

  async function writeClipboardText(text) {
    if (!text) {
      return;
    }
    try {
      await Clipboard.SetText(text);
      return;
    } catch {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text);
      }
    }
  }

  async function readClipboardText() {
    try {
      return await Clipboard.Text();
    } catch {
      if (navigator.clipboard?.readText) {
        return navigator.clipboard.readText();
      }
      return "";
    }
  }

  function getSelectionText() {
    return term?.getSelection?.() || "";
  }

  async function copySelection() {
    const selection = getSelectionText();
    if (!selection) {
      return;
    }
    await writeClipboardText(selection);
    closeContextMenu();
    focusTerminal();
  }

  async function pasteClipboard() {
    const text = await readClipboardText();
    closeContextMenu();
    focusTerminal();
    if (!text) {
      return;
    }
    onInput(text);
  }

  function positionContextMenu(clientX, clientY) {
    const maxX = Math.max(12, window.innerWidth - contextMenuWidth - 12);
    const maxY = Math.max(12, window.innerHeight - contextMenuHeight - 12);
    contextMenuX = Math.min(clientX, maxX);
    contextMenuY = Math.min(clientY, maxY);
  }

  function handleContextMenu(event) {
    event.preventDefault();
    if (event.target?.closest?.(".terminal-context-menu")) {
      return;
    }
    contextMenuCanCopy = Boolean(getSelectionText());
    positionContextMenu(event.clientX, event.clientY);
    contextMenuOpen = true;
  }

  function handlePointerDown(event) {
    if (!event.target?.closest?.(".terminal-context-menu")) {
      closeContextMenu();
    }
    focusTerminal();
  }

  function handleGlobalPointerDown(event) {
    if (event.target?.closest?.(".terminal-context-menu")) {
      return;
    }
    closeContextMenu();
  }

  function handleGlobalKeydown(event) {
    if (event.key === "Escape") {
      closeContextMenu();
    }
  }

  function handleCustomTerminalKey(event) {
    if (event.type !== "keydown") {
      return true;
    }
    if (event.ctrlKey && event.shiftKey && !event.altKey && event.code === "KeyC") {
      event.preventDefault();
      void copySelection();
      return false;
    }
    return true;
  }

  function flushOutput() {
    flushScheduled = false;
    flushFrame = 0;
    if (!term || pendingOutput.length === 0) {
      return;
    }
    const chunk = pendingOutput.join("");
    pendingOutput = [];
    term.write(chunk);
  }

  function scheduleFlush() {
    if (flushScheduled) {
      return;
    }
    flushScheduled = true;
    flushFrame = window.requestAnimationFrame(flushOutput);
  }

  export function write(data) {
    term?.write(data);
  }

  export function replaceBuffer(buffer) {
    if (!term) {
      return;
    }
    pendingOutput = [];
    if (flushFrame) {
      window.cancelAnimationFrame(flushFrame);
      flushFrame = 0;
    }
    flushScheduled = false;
    suppressTerminalInput = true;
    term.reset();
    term.write(buffer || "", () => {
      suppressTerminalInput = false;
    });
  }

  export function clear() {
    term?.clear();
  }

  export function focus() {
    term?.focus();
  }

  export function fit() {
    fitAddon?.fit();
  }

  export function getSize() {
    return {
      cols: term?.cols || 120,
      rows: term?.rows || 32,
    };
  }

  onMount(() => {
    term = new Terminal({
      cursorBlink: true,
      fontFamily: "Consolas, Monaco, monospace",
      fontSize: 14,
      scrollback: 2000,
      theme,
    });
    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(terminalElement);
    term.attachCustomKeyEventHandler(handleCustomTerminalKey);
    fitAddon.fit();
    focusTerminal();
    term.onData((data) => {
      if (suppressTerminalInput) {
        return;
      }
      onInput(data);
    });

    window.addEventListener("pointerdown", handleGlobalPointerDown);
    window.addEventListener("keydown", handleGlobalKeydown);
    window.addEventListener("blur", closeContextMenu);
    window.addEventListener("resize", closeContextMenu);

    onReady({
      write(data) {
        if (!data) {
          return;
        }
        pendingOutput.push(data);
        scheduleFlush();
      },
      replaceBuffer(buffer) {
        pendingOutput = [];
        if (flushFrame) {
          window.cancelAnimationFrame(flushFrame);
          flushFrame = 0;
        }
        flushScheduled = false;
        suppressTerminalInput = true;
        term?.reset();
        term?.write(buffer || "", () => {
          suppressTerminalInput = false;
        });
      },
      clear() {
        term?.clear();
      },
      reset() {
        pendingOutput = [];
        if (flushFrame) {
          window.cancelAnimationFrame(flushFrame);
          flushFrame = 0;
        }
        flushScheduled = false;
        suppressTerminalInput = false;
        term?.reset();
      },
      focus() {
        focusTerminal();
      },
      fit() {
        fitAddon?.fit();
      },
      getSize() {
        return {
          cols: term?.cols || 120,
          rows: term?.rows || 32,
        };
      },
    });

    return () => {
      onReady(null);
      pendingOutput = [];
      closeContextMenu();
      if (flushFrame) {
        window.cancelAnimationFrame(flushFrame);
      }
      flushScheduled = false;
      flushFrame = 0;
      suppressTerminalInput = false;
      window.removeEventListener("pointerdown", handleGlobalPointerDown);
      window.removeEventListener("keydown", handleGlobalKeydown);
      window.removeEventListener("blur", closeContextMenu);
      window.removeEventListener("resize", closeContextMenu);
      term?.dispose();
    };
  });
</script>

<div class="terminal-view-shell">
  <div
    id="terminal"
    bind:this={terminalElement}
    role="presentation"
    tabindex="-1"
    on:contextmenu={handleContextMenu}
    on:mousedown={handlePointerDown}
    on:click={handlePointerDown}
    on:keydown={focusTerminal}
  ></div>

  {#if contextMenuOpen}
    <div
      class="terminal-context-menu context-menu"
      style={`left:${contextMenuX}px;top:${contextMenuY}px;`}
      role="menu"
      aria-label="终端菜单"
    >
      <button
        class="context-menu-item terminal-context-item"
        type="button"
        disabled={!contextMenuCanCopy}
        on:click={() => void copySelection()}
      >
        <span>复制</span>
        <small>Ctrl+Shift+C</small>
      </button>
      <button
        class="context-menu-item terminal-context-item"
        type="button"
        on:click={() => void pasteClipboard()}
      >
        <span>粘贴</span>
        <small>Clipboard</small>
      </button>
    </div>
  {/if}
</div>

<style>
  .terminal-view-shell {
    position: relative;
    width: 100%;
    height: 100%;
    min-height: 0;
  }

  .terminal-context-menu {
    position: fixed;
    z-index: 70;
    min-width: 228px;
  }

  .terminal-context-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    width: 100%;
    text-align: left;
  }

  .terminal-context-item small {
    color: rgba(230, 235, 255, 0.55);
    font-size: 11px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .terminal-context-item:disabled {
    cursor: not-allowed;
    color: rgba(230, 235, 255, 0.35);
    background: transparent;
  }

  .terminal-context-item:disabled small {
    color: rgba(230, 235, 255, 0.22);
  }

  .terminal-context-item:disabled:hover {
    background: transparent;
  }
</style>
