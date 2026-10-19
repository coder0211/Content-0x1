import { Megaphone } from "lucide-react";
import type { RepositoryFactories } from "@/shared/lib/di";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";
import { HttpCampaignRepository } from "./infrastructure/HttpCampaignRepository";

export const routes: AppRoute[] = [
  {
    path: "campaigns",
    lazy: lazyPage(() => import("./presentation/pages/CampaignsPage")),
    handle: { crumb: "Campaign", permission: "brand.view" },
  },
  {
    path: "campaigns/new",
    lazy: lazyPage(() => import("./presentation/pages/NewCampaignPage")),
    handle: { crumb: "Tạo campaign", permission: "campaign.manage" },
  },
  {
    path: "campaigns/:campaignId",
    lazy: lazyPage(() => import("./presentation/pages/CampaignDetailPage")),
    handle: { crumb: "Chi tiết", permission: "brand.view" },
  },
];

export const navItems: NavItem[] = [
  { to: "/campaigns", label: "Campaign", icon: Megaphone, group: "work", order: 20, permission: "brand.view" },
];

export const repositories: RepositoryFactories = { campaign: () => new HttpCampaignRepository() };

// Public API for other modules (studios read the campaign + brief from ?campaign_id=).
export { useCampaign, useCampaigns } from "./application/useCampaigns";
export { studioLink } from "./domain/rules";
export type { Campaign, CampaignBrief, ContentPlan, PlanItem } from "./domain/types";
