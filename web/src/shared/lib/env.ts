export const env = {
  apiBaseUrl: (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? "/api/v1",
  mocking: import.meta.env.VITE_API_MOCKING === "true",
} as const;
