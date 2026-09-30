import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { CLIENT_CERT_HEADER, verifiedClientCert, withClientCert } from "./clientcert.ts";

const verified = { certPresented: "1", certVerified: "SUCCESS", certRFC9440: ":AAAA:" };

describe("verifiedClientCert", () => {
  it("returns the leaf when Cloudflare verified it", () => {
    assert.equal(verifiedClientCert({ tlsClientAuth: verified }), ":AAAA:");
  });
  it("returns nothing without a verified certificate", () => {
    assert.equal(verifiedClientCert(undefined), null);
    assert.equal(verifiedClientCert({}), null);
    assert.equal(verifiedClientCert({ tlsClientAuth: null }), null);
    assert.equal(verifiedClientCert({ tlsClientAuth: { ...verified, certPresented: "0" } }), null);
    assert.equal(verifiedClientCert({ tlsClientAuth: { ...verified, certVerified: "FAILED:unknown ca" } }), null);
    assert.equal(verifiedClientCert({ tlsClientAuth: { ...verified, certVerified: "NONE" } }), null);
    assert.equal(verifiedClientCert({ tlsClientAuth: { ...verified, certRFC9440: "" } }), null);
    assert.equal(verifiedClientCert({ tlsClientAuth: { ...verified, certRFC9440TooLarge: true } }), null);
  });
});

describe("withClientCert", () => {
  const forged = { [CLIENT_CERT_HEADER]: ":FORGED:", Authorization: "Bearer t" };

  it("drops a client-supplied header when there is no verified certificate", () => {
    const out = withClientCert(new Request("https://x/api", { headers: forged }), undefined);
    assert.equal(out.headers.get(CLIENT_CERT_HEADER), null);
    assert.equal(out.headers.get("Authorization"), "Bearer t");
  });
  it("replaces a client-supplied header with the verified certificate", () => {
    const out = withClientCert(new Request("https://x/api", { headers: forged }), { tlsClientAuth: verified });
    assert.equal(out.headers.get(CLIENT_CERT_HEADER), ":AAAA:");
  });
});
