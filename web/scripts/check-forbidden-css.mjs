#!/usr/bin/env node
// SPEC-05 §1.1 for stylesheets: no gradients, blur/glass, large shadows or neon effects in CSS.
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const ROOT = new URL("../src", import.meta.url).pathname;
const RULES = [
  [/gradient\s*\(/i, "gradient"],
  [/backdrop-filter/i, "backdrop-filter (glassmorphism)"],
  [/filter\s*:\s*blur/i, "blur filter"],
  [/@apply[^;]*\b(bg-gradient-|backdrop-blur|rounded-(2xl|3xl)|shadow-(lg|xl|2xl))/, "forbidden @apply class"],
  [/text-shadow/i, "text-shadow"],
];

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) yield* walk(p);
    else if (p.endsWith(".css")) yield p;
  }
}

let failures = 0;
for (const file of walk(ROOT)) {
  readFileSync(file, "utf8")
    .split("\n")
    .forEach((line, i) => {
      for (const [re, label] of RULES) {
        if (re.test(line)) {
          failures++;
          console.error(`${relative(process.cwd(), file)}:${i + 1}  SPEC-05 §1.1 forbids ${label}`);
        }
      }
    });
}
if (failures) process.exit(1);
