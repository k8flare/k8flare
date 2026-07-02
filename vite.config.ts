import { defineConfig } from "vite-plus";
export default defineConfig({
  // spikes/ is throwaway verification evidence (incl. a vendored upstream
  // fork), not maintained source — keep it out of the fmt/lint gates.
  fmt: {
    ignorePatterns: ["spikes/**"],
  },
  lint: {
    ignorePatterns: ["spikes/**"],
  },
});
