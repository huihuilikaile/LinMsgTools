import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import wails from "@wailsio/runtime/plugins/vite";

export default defineConfig({
  plugins: [svelte(), wails("./bindings")],
});
