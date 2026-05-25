<script>
  import { createEventDispatcher } from "svelte";

  export let activeTab = "connection";
  export let connected = false;
  export let sidebarCollapsed = false;
  export let statusMessage = "等待连接";
  export let statusTone = "idle";
  export let statusIdentityLabel = "未连接";

  const dispatch = createEventDispatcher();

  const tabs = [
    { id: "connection", glyph: "CN", index: "01", title: "连接管理", subtitle: "连接参数与服务器记录" },
    { id: "response", glyph: "IR", index: "02", title: "应急响应", subtitle: "一键采集主机信息" },
    { id: "terminal", glyph: "TY", index: "03", title: "交互终端", subtitle: "远程 PTY 会话" },
  ];
</script>

<aside class="nav-sidebar" class:collapsed={sidebarCollapsed}>
  <div class="nav-brand">
    <div class="brand-mark">LS</div>
    <div class="nav-brand-copy">
      <p class="brand-kicker">LinuxSafeTools</p>
      <h1>SSH Console</h1>
    </div>
    <button class="nav-collapse-btn" type="button" aria-label="收起侧边栏" title="收起侧边栏" on:click={() => dispatch("togglecollapse")}></button>
  </div>

  <nav class="nav-menu">
    {#each tabs as tab}
      <button
        class:active={activeTab === tab.id}
        class="nav-item"
        data-tab={tab.id}
        type="button"
        aria-label={tab.title}
        title={tab.title}
        on:click={() => dispatch("selecttab", tab.id)}
      >
        <span class="nav-glyph" aria-hidden="true">{tab.glyph}</span>
        <span class="nav-index">{tab.index}</span>
        <span class="nav-copy">
          <strong>{tab.title}</strong>
          <small>{tab.subtitle}</small>
        </span>
      </button>
    {/each}
  </nav>

  <section class="nav-status">
    <p class="section-label sidebar-connection-label">Connection</p>
    <div class="sidebar-connection-card" data-state={connected ? statusTone : "idle"}>
      <strong class="sidebar-connection-value">{connected ? statusIdentityLabel : "未连接"}</strong>
      <small class="sidebar-connection-meta">{connected ? "当前已连接" : statusMessage}</small>
      {#if connected}
        <button class="mini-btn sidebar-connection-action" type="button" on:click={() => dispatch("disconnect")}>断开连接</button>
      {/if}
    </div>
  </section>

  <div class="sidebar-footer">
    <button class="mini-btn settings-action" type="button" on:click={() => dispatch("opensettings")}>设置</button>
  </div>
</aside>
