import adapter from "@sveltejs/adapter-static";
import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [
    sveltekit({
      adapter: adapter({
        fallback: "404.html",
      }),
      version: {
        name: "growse-v0.21.0-sveltekit-3",
      },
    }),
  ],
});
