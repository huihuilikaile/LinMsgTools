<script>
  import { createEventDispatcher } from "svelte";

  export let toasts = [];

  const dispatch = createEventDispatcher();
</script>

<div class="toast-stack">
  {#each toasts as toast (toast.id)}
    <button class:leaving={toast.leaving} class={`toast visible toast-${toast.tone}`} type="button" on:click={() => dispatch("dismiss", toast.id)}>
      <span class="toast-text">{toast.message}</span>
    </button>
  {/each}
</div>
