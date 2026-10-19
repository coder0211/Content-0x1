import js from "@eslint/js";
import boundaries from "eslint-plugin-boundaries";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";
import tseslint from "typescript-eslint";

/**
 * SPEC-05 §1.1 anti-"AI look": forbidden Tailwind classes inside any string/template literal.
 * Tailwind's theme is also restricted (tailwind.config.ts) so most of these would not even render.
 */
const FORBIDDEN_CLASS =
  "(^|[\\s:\"'`])(bg-gradient-|bg-linear-|bg-radial-|from-[a-z]|via-[a-z]|backdrop-|blur-|rounded-(2xl|3xl|\\[)|shadow-(lg|xl|2xl|inner|\\[)|animate-(bounce|ping)|text-(xs|sm|base|lg|[2-9]?xl)\\b|font-(thin|extralight|light|bold|extrabold|black)\\b)";
const ROUNDED_FULL = "(^|[\\s:\"'`])rounded-full\\b";

const forbiddenClassRule = (patterns) => [
  "error",
  ...patterns.flatMap(([re, message]) => [
    { selector: `Literal[value=/${re}/]`, message },
    { selector: `TemplateElement[value.raw=/${re}/]`, message },
  ]),
];

const FORBIDDEN_MSG =
  "SPEC-05 §1.1: forbidden class (gradient, blur, rounded-2xl+, shadow-lg+, non-token font size/weight). Use design tokens.";
const ROUNDED_MSG = "SPEC-05 §1.1: rounded-full is only allowed in shared/ui (avatar, status dot, switch).";

export default tseslint.config(
  {
    ignores: ["dist", "coverage", "node_modules", "public", "playwright-report", "test-results", "src/shared/api/schema.d.ts"],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: { ecmaVersion: 2022, globals: { ...globals.browser } },
    plugins: { "react-hooks": reactHooks },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_", varsIgnorePattern: "^_" }],
      "@typescript-eslint/consistent-type-imports": ["error", { fixStyle: "inline-type-imports" }],
      "no-restricted-syntax": forbiddenClassRule([
        [FORBIDDEN_CLASS, FORBIDDEN_MSG],
        [ROUNDED_FULL, ROUNDED_MSG],
      ]),
    },
  },
  {
    files: ["src/shared/ui/**/*.{ts,tsx}"],
    rules: { "no-restricted-syntax": forbiddenClassRule([[FORBIDDEN_CLASS, FORBIDDEN_MSG]]) },
  },
  {
    // Architecture boundaries — SPEC-06 §3.2 / §3.4.
    files: ["src/**/*.{ts,tsx}"],
    ignores: ["src/**/*.test.{ts,tsx}", "src/test/**"],
    plugins: { boundaries },
    settings: {
      "import/resolver": { typescript: { project: "./tsconfig.app.json" } },
      "boundaries/include": ["src/**/*"],
      "boundaries/elements": [
        { type: "app", pattern: "src/app", mode: "folder" },
        { type: "shared-kernel", pattern: "src/shared/kernel", mode: "folder" },
        { type: "shared-lib", pattern: "src/shared/lib", mode: "folder" },
        { type: "shared-api", pattern: "src/shared/api", mode: "folder" },
        { type: "shared-ui", pattern: "src/shared/ui", mode: "folder" },
        { type: "module-index", pattern: "src/modules/*/index.ts", mode: "full", capture: ["module"] },
        { type: "domain", pattern: "src/modules/*/domain", mode: "folder", capture: ["module"] },
        { type: "application", pattern: "src/modules/*/application", mode: "folder", capture: ["module"] },
        { type: "infrastructure", pattern: "src/modules/*/infrastructure", mode: "folder", capture: ["module"] },
        { type: "presentation", pattern: "src/modules/*/presentation", mode: "folder", capture: ["module"] },
      ],
    },
    rules: {
      "boundaries/no-unknown-files": "error",
      "boundaries/element-types": [
        "error",
        {
          default: "disallow",
          message: "SPEC-06 §3.2: ${file.type} must not import ${dependency.type}",
          rules: [
            { from: "app", allow: ["*"] },
            { from: "shared-kernel", allow: ["shared-kernel"] },
            { from: "shared-lib", allow: ["shared-kernel", "shared-lib"] },
            { from: "shared-api", allow: ["shared-kernel", "shared-lib", "shared-api"] },
            { from: "shared-ui", allow: ["shared-kernel", "shared-lib", "shared-ui"] },
            {
              from: "module-index",
              allow: [
                "shared-kernel",
                "shared-lib",
                ["domain", { module: "${from.module}" }],
                ["application", { module: "${from.module}" }],
                ["infrastructure", { module: "${from.module}" }],
                ["presentation", { module: "${from.module}" }],
              ],
            },
            { from: "domain", allow: ["shared-kernel", ["domain", { module: "${from.module}" }]] },
            {
              from: "application",
              allow: [
                "shared-kernel",
                "shared-lib",
                ["domain", { module: "${from.module}" }],
                ["application", { module: "${from.module}" }],
                ["module-index", { module: "!${from.module}" }],
              ],
            },
            {
              from: "infrastructure",
              allow: [
                "shared-kernel",
                "shared-lib",
                "shared-api",
                ["domain", { module: "${from.module}" }],
                ["application", { module: "${from.module}" }],
                ["infrastructure", { module: "${from.module}" }],
              ],
            },
            {
              from: "presentation",
              allow: [
                "shared-kernel",
                "shared-lib",
                "shared-ui",
                ["domain", { module: "${from.module}" }],
                ["application", { module: "${from.module}" }],
                ["presentation", { module: "${from.module}" }],
                ["module-index", { module: "!${from.module}" }],
              ],
            },
          ],
        },
      ],
    },
  },
  {
    // Components and hooks never touch the network directly (SPEC-06 §3.2).
    files: ["src/modules/*/presentation/**/*.{ts,tsx}", "src/modules/*/application/**/*.{ts,tsx}", "src/modules/*/domain/**/*.ts"],
    ignores: ["src/**/*.test.{ts,tsx}"],
    rules: {
      "no-restricted-globals": [
        "error",
        { name: "fetch", message: "Call the API via a repository in infrastructure/." },
        { name: "EventSource", message: "Use shared/api/sse from infrastructure/." },
      ],
    },
  },
  {
    files: ["*.{js,ts}", "scripts/**/*.mjs", "e2e/**/*.ts"],
    languageOptions: { globals: { ...globals.node } },
  },
);
