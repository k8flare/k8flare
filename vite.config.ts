import { defineConfig } from "vite-plus";
export default defineConfig({
  // spikes/ is throwaway verification evidence; third_party/ is a vendored
  // upstream fork (syumai/workers, patched two files) -- reformatting either
  // wholesale would bury the real patch in reformat noise and make future
  // diffs against upstream unreadable. Keep both out of the fmt/lint gates.
  fmt: {
    ignorePatterns: ["spikes/**", "third_party/**"],
  },
  lint: {
    ignorePatterns: ["spikes/**", "third_party/**"],
  },
});
