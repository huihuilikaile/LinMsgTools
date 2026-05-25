  <script>
  import { createEventDispatcher } from "svelte";
  import { splitOutputLines, tokenizeOutputLine } from "../utils/format.js";

  export let active = false;
  export let quickCommands = [];
  export let shellFiles = [];
  export let selectedJSONShellFile = "";
  export let currentCommand = "";
  export let currentCommandTitle = "";
  export let resultTitle = "";
  export let resultCommand = "";
  export let resultOutput = "";
  export let quickCommandStatuses = {};
  export let overviewOpen = false;
  export let overviewResults = [];
  export let quickOutputLineNumbers = true;
  export let contextMenuOpen = false;
  export let contextMenuPosition = { x: 0, y: 0 };
  const dispatch = createEventDispatcher();

  $: outputLines = splitOutputLines(resultOutput);
</script>

<section class:active={active} class="tab-panel" data-panel="response">
  <section class="panel quick-panel">
    <aside class="quick-sidebar">
      <div class="quick-sidebar-head">
        <div>
          <h3>命令查询</h3>
        </div>
        <div class="quick-sidebar-actions">
          <button class="btn btn-secondary compact quick-run-all-btn" type="button" on:click={() => dispatch("runall")}>一键执行</button>
          <button class="btn btn-secondary compact quick-overview-btn" type="button" disabled={!overviewResults.length} on:click={() => dispatch("openoverview")}>总览</button>
        </div>
      </div>

      <div class="quick-toolbar">
        {#if !quickCommands.length}
          <div class="empty-state">正在加载命令配置...</div>
        {:else}
          {#each quickCommands as item}
            <button
              class:active={currentCommand === item.command}
              class:completed={quickCommandStatuses[item.id]?.status === "success"}
              class="quick-btn"
              type="button"
              data-command={item.id}
              on:click={() => dispatch("runquick", item)}
              on:contextmenu|preventDefault={(event) => {
                if (item.builtin || item.source) {
                  return;
                }
                dispatch("opencontext", {
                  command: item,
                  position: { x: event.clientX, y: event.clientY },
                });
              }}
            >
              <span class="quick-btn-head">
                <span class="quick-btn-title">{item.name}</span>
                <span class="quick-command-meta">
                  <span class="quick-command-badge">{item.source ? "扩展" : item.builtin ? "内置" : "自定义"}</span>
                </span>
              </span>
              <span class={`quick-command-status quick-command-status-${quickCommandStatuses[item.id]?.status || "idle"}`}>
                {quickCommandStatuses[item.id]?.message || (item.source ? item.source : "等待执行")}
              </span>
            </button>
          {/each}
        {/if}
      </div>
    </aside>

    <div class="quick-content">
      <div class="result-card">
        <div class="result-card-head">
          <div>
            <h3>命令操作 <span class="batch-count">{quickCommands.length ? `${quickCommands.length} 项` : ""}</span></h3>
          </div>
          <div class="result-card-actions">
            <select class="quick-shell-select" value={selectedJSONShellFile} on:change={(event) => dispatch("selectshell", event.currentTarget.value)}>
              <option value="">加载扩展命令</option>
              {#each shellFiles as item}
                <option value={item.fileName}>{item.label}</option>
              {/each}
            </select>

            <div class="quick-output-toggle" role="radiogroup" aria-label="输出行号显示">
              <label class="quick-output-toggle-option">
                <input name="quick-line-numbers" type="radio" checked={quickOutputLineNumbers} on:change={() => dispatch("togglenumbers", true)} />
                <span>显示行号</span>
              </label>
              <label class="quick-output-toggle-option">
                <input name="quick-line-numbers" type="radio" checked={!quickOutputLineNumbers} on:change={() => dispatch("togglenumbers", false)} />
                <span>隐藏行号</span>
              </label>
            </div>

            <button class="btn btn-secondary compact quick-add-btn" type="button" on:click={() => dispatch("openaddmodal")}>添加命令</button>
            <button class="btn btn-secondary compact quick-refresh-btn" type="button" on:click={() => dispatch("refresh")}>刷新</button>
          </div>
        </div>

        <div class="result-meta">
          {#if resultTitle}
            <strong>{resultTitle}</strong>
            {#if resultCommand}
              <span> · {resultCommand}</span>
            {/if}
          {:else}
            选择一项后在这里查看结果。
          {/if}
        </div>

        <div class="command-bar">
          <input
            class="command-input"
            type="text"
            placeholder="这里会显示可执行命令，可修改后再次执行"
            value={currentCommand}
            on:input={(event) => dispatch("updatemanual", { command: event.currentTarget.value, title: currentCommandTitle || resultTitle || "自定义命令" })}
          />
          <button class="btn btn-secondary compact" type="button" on:click={() => dispatch("runmanual")}>执行</button>
        </div>

        <div class:hide-line-numbers={!quickOutputLineNumbers} class="quick-output">
          {#each outputLines as line, index}
            <div class:without-line-no={!quickOutputLineNumbers} class="quick-output-line">
              <span class="quick-output-line-no">{String(index + 1).padStart(3, "0")}</span>
              <span class="quick-output-line-text">
                {#each tokenizeOutputLine(line || "\u00A0") as token}
                  <span class={`quick-output-token quick-output-token-${token.type}`}>{token.value}</span>
                {/each}
              </span>
            </div>
          {/each}
        </div>
      </div>
    </div>
  </section>

  {#if overviewOpen}
    <div class="modal-overlay">
      <section class="settings-modal quick-overview-modal">
        <header class="settings-header">
          <div>
            <h3>一键执行总览</h3>
            <p class="settings-note">共 {overviewResults.length} 项命令结果</p>
          </div>
          <div class="quick-overview-actions">
            <div class="quick-output-toggle" role="radiogroup" aria-label="总览输出行号显示">
              <label class="quick-output-toggle-option">
                <input name="overview-line-numbers" type="radio" checked={quickOutputLineNumbers} on:change={() => dispatch("togglenumbers", true)} />
                <span>显示行号</span>
              </label>
              <label class="quick-output-toggle-option">
                <input name="overview-line-numbers" type="radio" checked={!quickOutputLineNumbers} on:change={() => dispatch("togglenumbers", false)} />
                <span>隐藏行号</span>
              </label>
            </div>
            <button class="titlebar-btn settings-close overview-close" type="button" aria-label="关闭总览" on:click={() => dispatch("closeoverview")}>关闭 X</button>
          </div>
        </header>
        <div class="quick-overview-grid">
          {#each overviewResults as item (item.item.id)}
            <article class={`quick-overview-card quick-overview-card-${item.status}`}>
              <div class="quick-overview-card-head">
                <strong>{item.result?.title || item.item.name}</strong>
                <span>{item.message}</span>
              </div>
              <code>{item.result?.command || item.item.command}</code>
              <div class:hide-line-numbers={!quickOutputLineNumbers} class="quick-output quick-overview-output">
                {#each splitOutputLines(item.result?.output || "(无输出)") as line, index}
                  <div class:without-line-no={!quickOutputLineNumbers} class="quick-output-line">
                    <span class="quick-output-line-no">{String(index + 1).padStart(3, "0")}</span>
                    <span class="quick-output-line-text">
                      {#each tokenizeOutputLine(line || "\u00A0") as token}
                        <span class={`quick-output-token quick-output-token-${token.type}`}>{token.value}</span>
                      {/each}
                    </span>
                  </div>
                {/each}
              </div>
            </article>
          {/each}
        </div>
      </section>
    </div>
  {/if}

  {#if contextMenuOpen}
    <div
      class="context-menu"
      style={`left:${contextMenuPosition.x}px;top:${contextMenuPosition.y}px;`}
    >
      <button class="context-menu-item danger" type="button" on:click={() => dispatch("deletecommand")}>删除命令</button>
      <div class="context-menu-divider"></div>
      <div class="context-menu-group">
        <div class="context-menu-label">转到扩展</div>
        <div class="context-menu-sublist">
          {#each shellFiles as item}
            <button class="context-menu-item" type="button" on:click={() => dispatch("moveshell", item.fileName)}>{item.label}</button>
          {/each}
        </div>
        <button class="context-menu-item" type="button" on:click={() => dispatch("createshell")}>新建扩展 JSON</button>
      </div>
    </div>
  {/if}
</section>
