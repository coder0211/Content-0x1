import animate from "tailwindcss-animate";
import type { Config } from "tailwindcss";
import { fontSize, layout, palette, radius, shadow } from "./src/shared/ui/tokens";

/**
 * Theme is REPLACED (not extended) for colors, font sizes, weights, radius and shadows so that
 * only SPEC-05 tokens exist. Classes like `text-purple-500`, `text-2xl`, `rounded-2xl`, `shadow-lg`
 * simply do not generate CSS; `pnpm lint` additionally rejects them.
 */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    colors: {
      transparent: "transparent",
      current: "currentColor",
      white: "#FFFFFF",
      ...palette,
      background: "hsl(var(--background) / <alpha-value>)",
      foreground: "hsl(var(--foreground) / <alpha-value>)",
      card: "hsl(var(--card) / <alpha-value>)",
      border: "hsl(var(--border) / <alpha-value>)",
      input: "hsl(var(--input) / <alpha-value>)",
      ring: "hsl(var(--ring) / <alpha-value>)",
      primary: {
        DEFAULT: "hsl(var(--primary) / <alpha-value>)",
        foreground: "hsl(var(--primary-foreground) / <alpha-value>)",
      },
      destructive: "hsl(var(--destructive) / <alpha-value>)",
      "muted-foreground": "hsl(var(--muted-foreground) / <alpha-value>)",
    },
    fontSize: { ...fontSize },
    fontWeight: { normal: "400", medium: "500", semibold: "600" },
    fontFamily: {
      sans: ['"Be Vietnam Pro"', "Inter", "system-ui", "sans-serif"],
      mono: ['"JetBrains Mono"', "ui-monospace", "monospace"],
    },
    borderRadius: { none: "0", ...radius, full: "9999px" },
    boxShadow: { none: "none", ...shadow },
    extend: {
      width: { sidebar: layout.sidebar, "sidebar-collapsed": layout.sidebarCollapsed, drawer: layout.drawer },
      height: { topbar: layout.topbar },
      maxWidth: { editor: layout.editorMax },
      ringColor: { DEFAULT: palette.accent[600] },
      ringOffsetColor: { DEFAULT: palette.neutral[0] },
      transitionDuration: { DEFAULT: "150ms" },
      transitionTimingFunction: { DEFAULT: "ease-out" },
    },
  },
  plugins: [animate],
} satisfies Config;
