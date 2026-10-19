#!/usr/bin/env node
// Generates src/shared/api/schema.d.ts from the API's OpenAPI document (openapi-typescript).
// Tolerates absence of the spec so the web app builds before the API contract exists.
import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const candidates = [
  process.env.OPENAPI_SPEC,
  "../api/openapi/openapi.bundled.yaml",
  "../api/openapi/bundle.yaml",
  "../api/openapi/openapi.yaml",
  "../api/openapi.yaml",
]
  .filter(Boolean)
  .map((p) => resolve(webRoot, p));

const spec = candidates.find((p) => existsSync(p));
if (!spec) {
  console.warn(`[gen:api] No OpenAPI spec found (looked in: ${candidates.join(", ")}). Keeping existing schema.d.ts.`);
  process.exit(0);
}

const out = join(webRoot, "src/shared/api/schema.d.ts");
const bin = join(webRoot, "node_modules/.bin/openapi-typescript");
execFileSync(bin, [spec, "-o", out], { stdio: "inherit" });
console.log(`[gen:api] ${spec} → ${out}`);
