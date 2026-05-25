<script>
  import { createEventDispatcher } from "svelte";

  export let open = false;
  export let splashEnabled = true;
  export let launchInfoEnabled = false;

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
    <div class="settings-modal" role="dialog" aria-modal="true" aria-labelledby="settings-title">
      <header class="settings-header">
        <div>
          <p class="section-label">Settings</p>
          <h3 id="settings-title">工具信息</h3>
        </div>
        <button class="titlebar-btn settings-close" type="button" aria-label="关闭设置" on:click={() => dispatch("close")}>X</button>
      </header>

      <div class="settings-grid">
        <article class="settings-card">
          <p class="section-label">Tool</p>
          <h4>工具信息</h4>
          <dl class="settings-list">
            <div class="settings-row">
              <dt>名称</dt>
              <dd>LinuxSafeTools</dd>
            </div>
            <div class="settings-row">
              <dt>定位</dt>
              <dd>SSH 应急信息查询工具</dd>
            </div>
            <div class="settings-row">
              <dt>功能</dt>
              <dd>连接管理、应急命令、交互终端</dd>
            </div>
            <div class="settings-row">
              <dt>配置文件</dt>
              <dd>应用同目录 JSON</dd>
            </div>
            <div class="settings-row settings-row-toggle">
              <dt>启动动画</dt>
              <dd>
                <div class="inline-radio-group" role="radiogroup" aria-label="是否显示启动动画">
                  <label class="inline-radio-option settings-toggle-option">
                    <input name="settings-splash-enabled" type="radio" checked={splashEnabled} on:change={() => dispatch("toggleSplash", true)} />
                    <span>显示</span>
                  </label>
                  <label class="inline-radio-option settings-toggle-option">
                    <input name="settings-splash-enabled" type="radio" checked={!splashEnabled} on:change={() => dispatch("toggleSplash", false)} />
                    <span>关闭</span>
                  </label>
                </div>
              </dd>
            </div>
            <div class="settings-row settings-row-toggle">
              <dt>开屏说明窗</dt>
              <dd>
                <div class="inline-radio-group" role="radiogroup" aria-label="是否显示开屏说明窗">
                  <label class="inline-radio-option settings-toggle-option">
                    <input name="settings-launch-info-enabled" type="radio" checked={launchInfoEnabled} on:change={() => dispatch("toggleLaunchInfo", true)} />
                    <span>显示</span>
                  </label>
                  <label class="inline-radio-option settings-toggle-option">
                    <input name="settings-launch-info-enabled" type="radio" checked={!launchInfoEnabled} on:change={() => dispatch("toggleLaunchInfo", false)} />
                    <span>关闭</span>
                  </label>
                </div>
              </dd>
            </div>
          </dl>
        </article>

        <article class="settings-card">
          <p class="section-label">Profile</p>
          <h4>huihuilikaile</h4>
          <dl class="settings-list">
            <div class="settings-row">
              <dt>技术栈</dt>
              <dd>Svelte 5, Vite, Wails3, Go, xterm.js</dd>
            </div>
          </dl>
        </article>
      </div>
    </div>
  </div>
{/if}
