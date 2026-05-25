<script>
  import { createEventDispatcher } from "svelte";

  export let title = "LinuxSafeTools";

  const dispatch = createEventDispatcher();
</script>

<header class="titlebar">
  <div class="titlebar-left">
    <strong class="titlebar-name">{title}</strong>
  </div>
  <div class="titlebar-actions">
    <button id="minimize-btn" class="titlebar-btn" type="button" aria-label="最小化" on:click={() => dispatch("action", "minimise")}>_</button>
    <button id="maximize-btn" class="titlebar-btn" type="button" aria-label="最大化" on:click={() => dispatch("action", "maximise")}>[]</button>
    <button id="close-btn" class="titlebar-btn titlebar-btn-close" type="button" aria-label="关闭" on:click={() => dispatch("action", "close")}>X</button>
  </div>
</header>
