<script>
  import { createEventDispatcher } from "svelte";

  export let open = false;
  export let value = "";

  const dispatch = createEventDispatcher();
</script>

{#if open}
  <div
    class="modal-overlay"
    role="presentation"
    tabindex="-1"
    on:click={(event) => event.target === event.currentTarget && dispatch("close")}
    on:keydown={(event) => event.key === "Escape" && dispatch("close")}
  >
    <div class="settings-modal quick-add-modal" role="dialog" aria-modal="true" aria-labelledby="json-shell-title">
      <header class="settings-header">
        <div>
          <p class="section-label">扩展 JSON</p>
          <h3 id="json-shell-title">新建扩展 JSON</h3>
        </div>
        <button class="titlebar-btn settings-close" type="button" aria-label="关闭新建扩展 JSON" on:click={() => dispatch("close")}>X</button>
      </header>

      <div class="quick-add-body">
        <p class="settings-note">输入文件名后会在应用同目录的 jsonshell 文件夹中创建对应的 .json 文件。</p>
        <form class="quick-command-form" on:submit|preventDefault={() => dispatch("submit")}>
          <label class="field">
            <span>文件名</span>
            <input type="text" placeholder="例如: web_ops" value={value} on:input={(event) => dispatch("update", event.currentTarget.value)} />
          </label>

          <div class="settings-actions">
            <button class="btn btn-primary compact" type="submit">创建并转入</button>
          </div>
        </form>
      </div>
    </div>
  </div>
{/if}
