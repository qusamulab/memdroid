import assert from "node:assert/strict";
import { test } from "node:test";
import { api } from "../static/js/api.js";
import { actionArgs, escHtml } from "../static/js/ui.js";

test("table action arguments preserve quotes without becoming executable HTML", () => {
  const payload = `value'"><img src=x onerror=alert(1)> & \"quoted\"`;
  const encoded = actionArgs("0x1000", payload);
  assert.doesNotMatch(encoded, /[<>"']/);
  const decoded = encoded.replace(
    /&(amp|lt|gt|quot|#39);/g,
    (_, entity) =>
      ({
        amp: "&",
        lt: "<",
        gt: ">",
        quot: '"',
        "#39": "'",
      })[entity],
  );
  assert.deepEqual(JSON.parse(decoded), ["0x1000", payload]);
  assert.equal(escHtml("<&>"), "&lt;&amp;&gt;");
});

test("API transport distinguishes reads, writes, auth and network failures", async (t) => {
  const original = globalThis.fetch;
  t.after(() => {
    globalThis.fetch = original;
  });
  const calls = [];
  globalThis.fetch = async (...args) => {
    calls.push(args);
    return { ok: true, status: 200, json: async () => ({ ok: true }) };
  };
  assert.deepEqual(await api("/status"), { ok: true });
  assert.deepEqual(calls[0], ["/api/status", {}]);
  await api("/search/reset", {});
  assert.equal(calls[1][1].method, "POST");
  assert.equal(calls[1][1].body, "{}");

  globalThis.fetch = async () => ({
    ok: false,
    status: 400,
    json: async () => ({ error: "invalid value" }),
  });
  assert.deepEqual(await api("/search/value", {}), { error: "invalid value" });
  globalThis.fetch = async () => ({ ok: false, status: 401 });
  assert.match((await api("/status")).error, /Authentication required/);
  globalThis.fetch = async () => {
    throw new Error("offline");
  };
  assert.match((await api("/status")).error, /Cannot reach memdroid/);
});
