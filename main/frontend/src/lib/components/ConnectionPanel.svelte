<script>
  import { createEventDispatcher } from "svelte";

  export let active = false;
  export let form;
  export let serverRecords = [];
  export let currentRecordId = "";
  export let connectedRecordId = "";
  export let formatTime = (value) => value;

  const dispatch = createEventDispatcher();

  function updateField(key, value) {
    dispatch("updateform", { [key]: value });
  }
</script>

<section class:active={active} class="tab-panel" data-panel="connection">
  <div class="panel-grid">
    <form class="panel form-panel" on:submit|preventDefault={() => dispatch("connect")}>
      <div class="panel-top">
        <div>
          <p class="section-label">Profile</p>
          <h3>连接参数</h3>
        </div>
        <button class="btn btn-secondary compact" type="button" on:click={() => dispatch("saverecord")}>保存记录</button>
      </div>

      <div class="field-grid">
        <label class="field span-2">
          <span>记录名称</span>
          <input type="text" placeholder="生产 Web 01" value={form.serverName} on:input={(event) => updateField("serverName", event.currentTarget.value)} />
        </label>

        <label class="field span-2">
          <span>主机地址</span>
          <input type="text" placeholder="192.168.1.10" value={form.host} on:input={(event) => updateField("host", event.currentTarget.value)} />
        </label>

        <label class="field">
          <span>端口</span>
          <input type="number" value={form.port} on:input={(event) => updateField("port", Number(event.currentTarget.value) || 22)} />
        </label>

        <label class="field">
          <span>用户</span>
          <input type="text" placeholder="root" value={form.user} on:input={(event) => updateField("user", event.currentTarget.value)} />
        </label>

        <label class="field span-2">
          <span>密码</span>
          <input type="password" placeholder="密码认证时填写，可保存到服务器记录" value={form.password} on:input={(event) => updateField("password", event.currentTarget.value)} />
        </label>

        <label class="field span-2">
          <span>私钥路径</span>
          <input type="text" placeholder="C:\\Users\\me\\.ssh\\id_rsa" value={form.keyPath} on:input={(event) => updateField("keyPath", event.currentTarget.value)} />
        </label>

        <label class="field span-2">
          <span>备注</span>
          <input type="text" placeholder="堡垒机后方 / 生产环境" value={form.serverDescription} on:input={(event) => updateField("serverDescription", event.currentTarget.value)} />
        </label>

        <label class="field">
          <span>列数</span>
          <input type="number" value={form.cols} on:input={(event) => updateField("cols", Number(event.currentTarget.value) || 120)} />
        </label>

        <label class="field">
          <span>行数</span>
          <input type="number" value={form.rows} on:input={(event) => updateField("rows", Number(event.currentTarget.value) || 32)} />
        </label>

        <div class="field span-2">
          <span>记录日志</span>
          <div class="inline-radio-group" role="radiogroup" aria-label="是否记录连接日志">
            <label class="inline-radio-option">
              <input name="connect-logging" type="radio" checked={form.logging} on:change={() => updateField("logging", true)} />
              <span>记录</span>
            </label>
            <label class="inline-radio-option">
              <input name="connect-logging" type="radio" checked={!form.logging} on:change={() => updateField("logging", false)} />
              <span>不记录</span>
            </label>
          </div>
        </div>
      </div>

      <div class="action-row">
        <button class="btn btn-primary" type="submit">连接</button>
        <button class="btn btn-secondary" type="button" on:click={() => dispatch("disconnect")}>断开</button>
      </div>
    </form>

    <section class="panel record-panel">
      <div class="panel-top">
        <div>
          <p class="section-label">Saved</p>
          <h3>服务器记录</h3>
        </div>
      </div>

      <div class="record-list">
        {#if !serverRecords.length}
          <div class="empty-state">暂无保存的服务器记录。</div>
        {:else}
          {#each serverRecords as record}
            <article class:selected={record.id === currentRecordId} class:connected={record.id === connectedRecordId} class="record-item" data-record-id={record.id}>
              <div class="record-main">
                <div class="record-title-row">
                  <strong>{record.name || `${record.user}@${record.host}`}</strong>
                  <span class="record-time">{formatTime(record.lastUsedAt)}</span>
                </div>
                <div class="record-line">{record.user}@{record.host}:{record.port}</div>
                <div class="record-line muted">{record.description || record.keyPath || (record.password ? "已保存密码" : "无备注")}</div>
              </div>
              <div class="record-actions">
                <button class="mini-btn" type="button" on:click={() => dispatch("userecord", record)}>填充</button>
                <button class="mini-btn" type="button" on:click={() => dispatch("connectrecord", record)}>连接</button>
                <button class="mini-btn" type="button" on:click={() => dispatch("deleterecord", { id: record.id })}>删除</button>
              </div>
            </article>
          {/each}
        {/if}
      </div>
    </section>
  </div>
</section>
