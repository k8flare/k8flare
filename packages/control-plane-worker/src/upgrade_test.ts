import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { refuseUpgradeAfterAdmission } from "./upgrade.ts";

describe("refuseUpgradeAfterAdmission", () => {
  it("returns the admission denial so the client reports it instead of the upgrade error", async () => {
    const denied = Response.json({ kind: "Status", code: 403, message: "attaching to pod 'p' is not allowed" }, { status: 403 });
    const got = await refuseUpgradeAfterAdmission(async () => denied);
    assert.equal(got.status, 403);
    assert.match(((await got.json()) as { message: string }).message, /not allowed/);
  });
  it("refuses with 426 when admission allowed the connect", async () => {
    const got = await refuseUpgradeAfterAdmission(async () => Response.json({ node: "n" }));
    assert.equal(got.status, 426);
    assert.equal(got.headers.get("Upgrade"), "websocket");
    assert.match(((await got.json()) as { message: string }).message, /only WebSocket upgrades/);
  });
});
