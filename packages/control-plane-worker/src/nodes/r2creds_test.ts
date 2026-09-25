import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { r2PodEnv } from "./r2creds.ts";

describe("r2 local mint", () => {
  it("returns empty env without account/bucket", async () => {
    const env = await r2PodEnv({} as Env, "ns", "pod");
    assert.deepEqual(env, {});
  });
});
