/**
 * Design tokens (SPEC-05 §2) — single source of truth for Tailwind config and the /_design page.
 * Do not add colors here without updating SPEC-05.
 */
export const palette = {
  neutral: {
    0: "#FFFFFF",
    25: "#FCFBFA",
    50: "#FAFAF9",
    100: "#F5F5F4",
    200: "#E7E5E4",
    300: "#D6D3D1",
    400: "#A8A29E",
    500: "#78716C",
    600: "#57534E",
    700: "#44403C",
    900: "#1C1917",
  },
  accent: {
    50: "#EEF3FE",
    100: "#DCE6FD",
    600: "#1F4FD8",
    700: "#1A40B0",
  },
  success: { DEFAULT: "#15803D", bg: "#F0FDF4", border: "#BBF7D0" },
  warning: { DEFAULT: "#B45309", bg: "#FFFBEB", border: "#FDE68A" },
  danger: { DEFAULT: "#B91C1C", bg: "#FEF2F2", border: "#FECACA" },
  info: { DEFAULT: "#1F4FD8", bg: "#EEF3FE", border: "#DCE6FD" },
  muted: { DEFAULT: "#57534E", bg: "#F5F5F4", border: "#E7E5E4" },
  video: { bg: "#0C0A09", fg: "#E7E5E4" },
} as const;

export const fontSize = {
  page: ["20px", { lineHeight: "28px", fontWeight: "600" }],
  section: ["16px", { lineHeight: "24px", fontWeight: "600" }],
  body: ["14px", { lineHeight: "20px" }],
  small: ["13px", { lineHeight: "18px" }],
  caption: ["12px", { lineHeight: "16px", fontWeight: "500" }],
  editor: ["16px", { lineHeight: "26px" }],
} as const satisfies Record<string, [string, { lineHeight: string; fontWeight?: string }]>;

export const radius = { sm: "4px", DEFAULT: "6px", lg: "8px" } as const;

export const shadow = {
  sm: "0 1px 2px rgb(28 25 23 / .06)",
  md: "0 4px 12px rgb(28 25 23 / .08)",
} as const;

export const motion = { fast: "150ms", base: "200ms" } as const;

export const layout = {
  sidebar: "232px",
  sidebarCollapsed: "56px",
  topbar: "48px",
  studioBrief: "320px",
  studioInspector: "300px",
  drawer: "480px",
  reviewList: "360px",
  editorMax: "720px",
} as const;
