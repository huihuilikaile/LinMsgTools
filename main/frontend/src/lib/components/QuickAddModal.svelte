<script>
  import { createEventDispatcher } from "svelte";

  export let open = false;
  export let form = { name: "", command: "" };

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
    <div class="settings-modal quick-add-modal" role="dialog" aria-modal="true" aria-labelledby="quick-add-title">
      <header class="settings-header">
        <div>
          <p class="section-label">Commands</p>
          <h3 id="quick-add-title">添加命令</h3>
        </div>
        <button class="titlebar-btn settings-close" type="button" aria-label="关闭添加命令" on:click={() => dispatch("close")}>X</button>
      </header>

      <div class="quick-add-body">
        <p class="settings-note">新增命令会作为自定义命令出现在左侧列表中，可通过右键删除。</p>
        <form class="quick-command-form" on:submit|preventDefault={() => dispatch("submit")}>
          <label class="field">
            <span>命令名称</span>
            <input type="text" placeholder="例如: 磁盘占用" value={form.name} on:input={(event) => dispatch("update", { name: event.currentTarget.value })} />
          </label>

          <label class="field">
            <span>命令内容</span>
            <textarea rows="6" placeholder="例如: df -h" value={form.command} on:input={(event) => dispatch("update", { command: event.currentTarget.value })}></textarea>
          </label>

          <div class="settings-actions">
            <button class="btn btn-primary compact" type="submit">添加命令</button>
          </div>
        </form>
      </div>
    </div>
  </div>
{/if}
