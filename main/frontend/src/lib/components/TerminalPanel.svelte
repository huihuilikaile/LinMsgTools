<script>
  import { tick } from "svelte";
  import TerminalView from "./TerminalView.svelte";

  export let active = true;
  export let mode = "main";
  export let detached = false;
  export let connected = false;
  export let statusMessage = "等待连接";
  export let terminalApi = null;
  export let sessions = [];
  export let activeSessionId = "";
  export let onTerminalReady = () => {};
  export let onTerminalInput = () => {};
  export let onClearTerminal = () => {};
  export let onSelectSession = () => {};
  export let onCloseSession = () => {};
  export let onDetach = () => {};
  export let onReturnToMain = () => {};

  let terminalMounted = mode !== "main";
  let tabRail;

  $: if (mode !== "main" || active) {
    terminalMounted = true;
  }
  $: activeSession = sessions.find((session) => session.id === activeSessionId) || null;
  $: showMainTerminalActions = !activeSession || activeSession.status !== "disconnected";

  function focusTerminal() {
    terminalApi?.focus();
  }

  function clearTerminal() {
    onClearTerminal();
  }

  async function selectSession(id) {
    await onSelectSession(id);
    await tick();
    terminalApi?.focus();
  }

  function scrollTabs(direction) {
    const amount = 240 * direction;
    tabRail?.scrollBy?.({
      left: amount,
      behavior: "smooth",
    });
  }

  function sessionUserName(session) {
    return session?.userName || String(session?.label || "--").split("@")[0] || "--";
  }
</script>

{#if mode === "main"}
  <section class:active={active} class="tab-panel terminal-panel" data-panel="terminal">
    <div class="terminal-shell terminal-shell-main">
      <div class="terminal-tabs terminal-tabs-top">
        <button class="terminal-tab-arrow" type="button" aria-label="向左查看终端标签" on:click={() => scrollTabs(-1)}>&lt;</button>
        <div class="terminal-tab-rail" bind:this={tabRail}>
          {#if sessions.length}
            {#each sessions as session (session.id)}
              <div
                class:active={session.id === activeSessionId}
                class:connected={session.status === "connected"}
                class:connecting={session.status === "connecting"}
                class:disconnected={session.status === "disconnected"}
                class="terminal-tab-card"
                title={`${session.label} · ${session.meta}`}
              >
                <button
                  class="terminal-tab-close"
                  type="button"
                  aria-label={`关闭 ${sessionUserName(session)} 标签`}
                  on:click|stopPropagation={() => onCloseSession(session.id)}
                >
                  x
                </button>
                <button
                  class="terminal-tab-face"
                  type="button"
                  on:click={() => selectSession(session.id)}
                >
                  <span class="terminal-tab-user">{sessionUserName(session)}</span>
                </button>
              </div>
            {/each}
          {:else}
            <div class="terminal-tab-empty">{connected ? "正在准备终端标签..." : "建立连接后会在这里显示终端标签"}</div>
          {/if}
        </div>
        <button class="terminal-tab-arrow" type="button" aria-label="向右查看终端标签" on:click={() => scrollTabs(1)}>&gt;</button>
      </div>

      <div class="terminal-topline">
        <div class="terminal-topline-status">
          <span class="terminal-dot"></span>
          <span class="terminal-dot"></span>
          <span class="terminal-dot"></span>
          <div class="terminal-caption">{statusMessage}</div>
        </div>
        {#if showMainTerminalActions}
          <div class="terminal-topline-actions terminal-topline-actions-main">
            <button class="btn btn-secondary compact" type="button" on:click={clearTerminal}>清屏</button>
            <button class="btn btn-secondary compact" type="button" on:click={focusTerminal}>聚焦</button>
            <button class="btn btn-secondary compact" type="button" on:click={onDetach}>{detached ? "聚焦独立终端" : "打开独立终端"}</button>
          </div>
        {/if}
      </div>

      <div class="terminal-frame">
        {#if terminalMounted}
          <TerminalView onReady={onTerminalReady} onInput={onTerminalInput} />
        {/if}
      </div>
    </div>
  </section>
{:else}
  <section class="terminal-shell terminal-shell-standalone">
    <div class="terminal-topline">
      <div class="terminal-topline-status">
        <span class="terminal-dot"></span>
        <span class="terminal-dot"></span>
        <span class="terminal-dot"></span>
        <div class="terminal-caption">{statusMessage}</div>
      </div>
      <div class="terminal-topline-actions">
        <button class="btn btn-secondary compact" type="button" on:click={clearTerminal}>清屏</button>
        <button class="btn btn-secondary compact" type="button" on:click={focusTerminal}>聚焦</button>
        <button class="btn btn-secondary compact" type="button" on:click={onReturnToMain}>返回主窗口</button>
      </div>
    </div>
    <div class="terminal-frame">
      <TerminalView onReady={onTerminalReady} onInput={onTerminalInput} />
    </div>
  </section>
{/if}
