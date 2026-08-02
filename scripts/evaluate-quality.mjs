import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");
const qualityDir = join(root, "analyze", "quality");
const scenarios = JSON.parse(readFileSync(join(qualityDir, "scenarios.json"), "utf8"));
const home = process.env.WOWDOC_HOME;
if (!home) throw new Error("WOWDOC_HOME is required so quality data stays isolated");
const suffix = process.platform === "win32" ? ".exe" : "";
const wowdoc = process.env.WOWDOC_BIN || join(root, "dist", "wowdoc" + suffix);
if (!existsSync(wowdoc)) throw new Error(`wowdoc binary not found: ${wowdoc}`);

const listCache = new Map();
const results = [];
for (const [index, scenario] of scenarios.entries()) {
  const listKey = `${scenario.source}:${scenario.product}`;
  let sourceList = listCache.get(listKey);
  if (!sourceList) {
    sourceList = runJSON(["source", "list", "--source", scenario.source, "--product", scenario.product]);
    listCache.set(listKey, sourceList);
  }
  const tags = sourceList.data?.tags ?? [];
  let ref = scenario.ref ?? "latest";
  if (Number.isInteger(scenario.tagIndex)) {
    if (tags.length === 0) {
      results.push(failedWithoutQuery(scenario, "no catalog Tag is available for this product branch"));
      continue;
    }
    const selected = tags[Math.min(scenario.tagIndex, tags.length - 1)];
    ref = selected.name ?? selected.Name;
  }
  process.stderr.write(`[${index + 1}/${scenarios.length}] ${scenario.id} ref=${ref}\n`);
  let envelope = runJSON(queryArgs(scenario, ref), true);
  if (!envelope.ok && envelope.error?.code === "snapshot_not_ready") {
    const build = runJSON(["index", "build", "--source", scenario.source, "--product", scenario.product, "--ref", ref], true, 30 * 60 * 1000);
    if (!build.ok) {
      results.push(failedWithoutQuery(scenario, `index build failed: ${build.error?.code ?? "unknown"}`, ref));
      continue;
    }
    envelope = runJSON(queryArgs(scenario, ref), true);
  }
  results.push(evaluate(scenario, ref, envelope));
}

const summary = summarize(results);
const artifact = { schema: "wowdoc.quality.v1", generatedAt: new Date().toISOString(), home, summary, results };
mkdirSync(qualityDir, { recursive: true });
writeFileSync(join(qualityDir, "results.json"), JSON.stringify(artifact, null, 2));
writeFileSync(join(qualityDir, "report.md"), markdown(artifact));
process.stdout.write(JSON.stringify(summary, null, 2) + "\n");
process.exit(summary.passed === summary.total ? 0 : 1);

function queryArgs(s, ref) {
  return ["query", "--source", s.source, "--product", s.product, "--ref", ref, "--topic", s.topic, "--text", s.query, "--limit", "5"];
}
function runJSON(args, allowFailure = false, timeout = 120000) {
  const child = spawnSync(wowdoc, args, { cwd: root, env: process.env, encoding: "utf8", timeout, windowsHide: true });
  let parsed;
  try { parsed = JSON.parse(child.stdout); } catch { parsed = { ok: false, error: { code: "invalid_json", message: child.stdout || child.stderr } }; }
  if (!allowFailure && (!parsed.ok || child.status !== 0)) throw new Error(`${args.join(" ")}: ${child.stderr || child.stdout}`);
  return parsed;
}
function evaluate(scenario, ref, envelope) {
  const top = envelope.data?.results?.[0];
  if (!envelope.ok || !top) return failedWithoutQuery(scenario, envelope.error?.code ?? "no top reference", ref);
  const expectedName = scenario.expectedName.toLowerCase();
  const correctness = (top.name ?? "").toLowerCase() === expectedName || top.excerpt.toLowerCase().includes(expectedName);
  const relevance = top.path.toLowerCase().includes(scenario.expectedPath.toLowerCase());
  const contextComplete = scenario.context.every(term => top.excerpt.toLowerCase().includes(term.toLowerCase()));
  const version = envelope.data.resolvedCommit && envelope.data.resolvedCommit.length === 40;
  const integrity = verifyGitEvidence(scenario.source, envelope.data.resolvedCommit, top);
  const dimensions = { correctness, relevance, contextComplete, version, traceability: integrity.ok };
  const score = Object.values(dimensions).filter(Boolean).length * 20;
  return { id: scenario.id, question: scenario.question, source: scenario.source, product: scenario.product, ref, query: scenario.query, resolvedCommit: envelope.data.resolvedCommit, matchedTag: envelope.data.matchedTag, top, dimensions, score, passed: score === 100, evidenceDiagnostic: integrity.message };
}
function verifyGitEvidence(source, commit, top) {
  try {
    const mirror = join(home, "repositories", source + ".git");
    const blob = execFileSync("git", ["--git-dir", mirror, "show", `${commit}:${top.path}`], { encoding: "buffer", maxBuffer: 64 * 1024 * 1024 });
    const hash = createHash("sha256").update(blob).digest("hex");
    if (hash !== top.contentHash) return { ok: false, message: "content hash differs from Git blob" };
    const lines = blob.toString("utf8").split(/\r?\n/);
    for (const row of top.excerpt.split("\n")) {
      const match = row.match(/^(\d+): (.*)$/s);
      if (!match || lines[Number(match[1]) - 1] !== match[2]) return { ok: false, message: `excerpt differs at ${match?.[1] ?? "unknown line"}` };
    }
    return { ok: true, message: "path, line, excerpt and SHA-256 match the resolved Commit blob" };
  } catch (error) { return { ok: false, message: String(error.message ?? error) }; }
}
function failedWithoutQuery(scenario, reason, ref = scenario.ref ?? null) {
  return { id: scenario.id, question: scenario.question, source: scenario.source, product: scenario.product, ref, query: scenario.query, dimensions: { correctness:false,relevance:false,contextComplete:false,version:false,traceability:false }, score: 0, passed: false, evidenceDiagnostic: reason };
}
function summarize(items) {
  const dimensions = ["correctness","relevance","contextComplete","version","traceability"];
  const summary = { total: items.length, passed: items.filter(x=>x.passed).length, averageScore: Math.round(items.reduce((n,x)=>n+x.score,0)/items.length), dimensions: {} };
  for (const name of dimensions) summary.dimensions[name] = items.filter(x=>x.dimensions[name]).length;
  summary.failed = summary.total - summary.passed;
  return summary;
}
function markdown(artifact) {
  const s=artifact.summary;const rows=["# wowdoc code-reference quality report","",`Generated: ${artifact.generatedAt}`,"",`Strict pass: ${s.passed}/${s.total}; average score: ${s.averageScore}/100.`,"",`Dimensions: correctness ${s.dimensions.correctness}/${s.total}, relevance ${s.dimensions.relevance}/${s.total}, context completeness ${s.dimensions.contextComplete}/${s.total}, version ${s.dimensions.version}/${s.total}, traceability ${s.dimensions.traceability}/${s.total}.`,"","| ID | Product | Ref | Score | Top reference | Result |","| --- | --- | --- | ---: | --- | --- |"];for(const item of artifact.results){const top=item.top?`${item.top.path}:${item.top.line}`:item.evidenceDiagnostic;rows.push(`| ${item.id} | ${item.source}/${item.product} | ${item.ref??""} | ${item.score} | ${String(top).replaceAll("|","\\|")} | ${item.passed?"PASS":"REVIEW"} |`)};rows.push("","A strict pass requires the first code reference to match the expected fact and subsystem, include the required answer context, resolve to an immutable Commit, and reproduce the exact Git blob bytes at the reported path and lines.","");return rows.join("\n")
}
