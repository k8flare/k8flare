import { defineConfig } from "vite-plus";
export default defineConfig({
  // spikes/ is throwaway verification evidence; third_party/ is a vendored
  // upstream fork (syumai/workers, patched two files) -- reformatting either
  // wholesale would bury the real patch in reformat noise and make future
  // diffs against upstream unreadable. Keep both out of the fmt/lint gates.
  fmt: {
    // Markdown is excluded because vp's md formatter corrupts prose, not
    // just reflows it (verified 2026-07-11 on docs/platform-verification.md:
    // AF_PACKET became AF*PACKET, "*compile*" became "\_compile*", and a
    // "+ flannel" continuation line was rewritten into a bullet list) --
    // and it mis-indents Japanese CJK text in CLAUDE.md's numbered lists.
    ignorePatterns: ["spikes/**", "third_party/**", "**/*.md"],
  },
  lint: {
    ignorePatterns: ["spikes/**", "third_party/**"],
  },
});
