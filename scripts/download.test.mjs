import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { mkdtemp } from "node:fs/promises";
import { downloadRelease } from "./download.mjs";
import { platformKey, releaseAsset } from "./platform.mjs";

const version = "9.9.9-test";
const asset = releaseAsset(version);
const body = Buffer.from("verified wowdoc fixture\n");
const checksum = createHash("sha256").update(body).digest("hex");

test("downloads, reports progress, and verifies SHA-256", async () => {
  const cache = await mkdtemp(join(tmpdir(), "wowdoc-download-"));
  const logs = [];
  await withCache(cache, async () => {
    const target = await downloadRelease({ version, attempts: 1, retryBaseMs: 1, stderr: logger(logs), fetchImpl: basicFetch() });
    assert.deepEqual(readFileSync(target), body);
    const cached = await downloadRelease({ version, attempts: 1, retryBaseMs: 1, stderr: logger(logs), fetchImpl: async () => { throw new Error("network should not be used"); } });
    assert.equal(cached, target);
  });
  assert.match(logs.join(""), /download .*attempt 1\/1/);
  assert.match(logs.join(""), /verified .*sha256=/);
});

test("resumes a partial response with a valid Content-Range", async () => {
  const cache = await mkdtemp(join(tmpdir(), "wowdoc-resume-"));
  const offset = 7;
  const part = join(cache, version, platformKey(), `${asset}.part`);
  mkdirSync(join(cache, version, platformKey()), { recursive: true });
  writeFileSync(part, body.subarray(0, offset));
  let range;
  await withCache(cache, async () => {
    const target = await downloadRelease({
      version,
      attempts: 1,
      retryBaseMs: 1,
      stderr: logger([]),
      fetchImpl: async (url, options = {}) => {
        if (url.endsWith("SHA256SUMS")) return checksumResponse();
        range = options.headers?.Range;
        return new Response(body.subarray(offset), { status: 206, headers: { "content-length": String(body.length - offset), "content-range": `bytes ${offset}-${body.length - 1}/${body.length}` } });
      },
    });
    assert.deepEqual(readFileSync(target), body);
  });
  assert.equal(range, `bytes=${offset}-`);
});

test("restarts safely when the server ignores Range", async () => {
  const cache = await mkdtemp(join(tmpdir(), "wowdoc-range-reset-"));
  const part = join(cache, version, platformKey(), `${asset}.part`);
  mkdirSync(join(cache, version, platformKey()), { recursive: true });
  writeFileSync(part, body.subarray(0, 5));
  await withCache(cache, async () => {
    const target = await downloadRelease({ version, attempts: 1, retryBaseMs: 1, stderr: logger([]), fetchImpl: basicFetch() });
    assert.deepEqual(readFileSync(target), body);
  });
});

test("retries a transient HTTP failure", async () => {
  const cache = await mkdtemp(join(tmpdir(), "wowdoc-retry-"));
  let assetRequests = 0;
  await withCache(cache, async () => {
    const target = await downloadRelease({
      version,
      attempts: 2,
      retryBaseMs: 1,
      stderr: logger([]),
      fetchImpl: async url => {
        if (url.endsWith("SHA256SUMS")) return checksumResponse();
        assetRequests++;
        if (assetRequests === 1) return new Response("busy", { status: 503 });
        return new Response(body, { status: 200, headers: { "content-length": String(body.length) } });
      },
    });
    assert.deepEqual(readFileSync(target), body);
  });
  assert.equal(assetRequests, 2);
});

test("serializes concurrent installers and reuses the verified cache", async () => {
  const cache = await mkdtemp(join(tmpdir(), "wowdoc-concurrent-"));
  const logs = [];
  let assetRequests = 0;
  const fetchImpl = async url => {
    if (url.endsWith("SHA256SUMS")) return checksumResponse();
    assetRequests++;
    await new Promise(resolve => setTimeout(resolve, 50));
    return new Response(body, { status: 200, headers: { "content-length": String(body.length) } });
  };
  await withCache(cache, async () => {
    const options = { version, attempts: 1, retryBaseMs: 1, totalMs: 5_000, stderr: logger(logs), fetchImpl };
    const [first, second] = await Promise.all([downloadRelease(options), downloadRelease(options)]);
    assert.equal(first, second);
    assert.deepEqual(readFileSync(first), body);
  });
  assert.equal(assetRequests, 1);
  assert.match(logs.join(""), /waiting for another installer/);
});

test("rejects content that does not match the release checksum", async () => {
  const cache = await mkdtemp(join(tmpdir(), "wowdoc-checksum-"));
  await withCache(cache, async () => {
    await assert.rejects(() => downloadRelease({
      version,
      attempts: 1,
      retryBaseMs: 1,
      stderr: logger([]),
      fetchImpl: async url => url.endsWith("SHA256SUMS")
        ? checksumResponse()
        : new Response(Buffer.from("corrupt"), { status: 200, headers: { "content-length": "7" } }),
    }), /checksum_mismatch/);
  });
});

test("aborts a response that stops making progress", async () => {
  const cache = await mkdtemp(join(tmpdir(), "wowdoc-idle-"));
  await withCache(cache, async () => {
    await assert.rejects(() => downloadRelease({
      version,
      attempts: 1,
      attemptMs: 500,
      idleMs: 20,
      totalMs: 1_000,
      retryBaseMs: 1,
      stderr: logger([]),
      fetchImpl: async (url, options = {}) => {
        if (url.endsWith("SHA256SUMS")) return checksumResponse();
        const stream = new ReadableStream({
          start(controller) {
            options.signal.addEventListener("abort", () => controller.error(options.signal.reason), { once: true });
          },
        });
        return new Response(stream, { status: 200 });
      },
    }), /download_idle_timeout/);
  });
});

test("aborts when response headers exceed the connection timeout", async () => {
  const cache = await mkdtemp(join(tmpdir(), "wowdoc-connect-"));
  await withCache(cache, async () => {
    await assert.rejects(() => downloadRelease({
      version,
      attempts: 1,
      connectMs: 20,
      attemptMs: 500,
      totalMs: 1_000,
      retryBaseMs: 1,
      stderr: logger([]),
      fetchImpl: async (_url, options = {}) => new Promise((_, reject) => {
        options.signal.addEventListener("abort", () => reject(options.signal.reason), { once: true });
      }),
    }), /download_connection_timeout/);
  });
});

test("uses carriage-return updates for TTY progress", async () => {
  const cache = await mkdtemp(join(tmpdir(), "wowdoc-tty-"));
  const logs = [];
  await withCache(cache, async () => {
    await downloadRelease({ version, attempts: 1, retryBaseMs: 1, stderr: logger(logs, true), fetchImpl: basicFetch() });
  });
  const output = logs.join("");
  assert.match(output, /\rwowdoc: download /);
  assert.match(output, /\rwowdoc: verified /);
  assert.equal(output.split("\n").length - 1, 1);
});

function basicFetch() {
  return async url => url.endsWith("SHA256SUMS")
    ? checksumResponse()
    : new Response(body, { status: 200, headers: { "content-length": String(body.length) } });
}

function checksumResponse() {
  return new Response(`${checksum}  dist/${asset}\n`, { status: 200 });
}

function logger(rows, isTTY = false) {
  return { isTTY, write(value) { rows.push(String(value)); } };
}

async function withCache(path, callback) {
  const previous = process.env.WOWDOC_CACHE_DIR;
  process.env.WOWDOC_CACHE_DIR = path;
  try { return await callback(); }
  finally {
    if (previous === undefined) delete process.env.WOWDOC_CACHE_DIR;
    else process.env.WOWDOC_CACHE_DIR = previous;
  }
}
