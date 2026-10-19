export const videoKeys = {
  all: ["video"] as const,
  list: (campaignId: string | null) => ["video", "list", campaignId ?? "all"] as const,
  project: (id: string) => ["video", "project", id] as const,
  estimate: (id: string) => ["video", "estimate", id] as const,
  brands: ["video", "brands"] as const,
  music: (brandId: string) => ["video", "music", brandId] as const,
};
