import type { ID } from "@/shared/kernel";
import type { CampaignAssetType } from "../domain/types";
import type { CampaignQuery } from "./ports";

export const campaignKeys = {
  all: ["campaign"] as const,
  lists: ["campaign", "list"] as const,
  list: (q: CampaignQuery) => ["campaign", "list", q] as const,
  detail: (id: ID) => ["campaign", "detail", id] as const,
  candidates: ["campaign", "member-candidates"] as const,
  assets: (id: ID, type: CampaignAssetType) => ["campaign", "assets", id, type] as const,
};
