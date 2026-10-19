import { Wallet } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";
import type { RepositoryFactories } from "@/shared/lib/di";
import { HttpBudgetRepository } from "./infrastructure/HttpBudgetRepository";

/** Public API for other modules (studios, campaign detail). */
export { useEstimate } from "./application/useEstimate";
export type { BudgetRepository } from "./application/ports";
export type { Budget, BudgetLimits, Estimate, EstimateKind, EstimateLine, EstimateRequest } from "./domain/budget";
export { EstimateBudgetBanner } from "./presentation/components/EstimateBudgetBanner";
export { CampaignBudgetDialog, type CampaignBudgetDialogProps } from "./presentation/components/CampaignBudgetDialog";

export const routes: AppRoute[] = [
  {
    path: "usage",
    lazy: lazyPage(() => import("./presentation/pages/UsagePage")),
    handle: { crumb: "Chi phí & ngân sách", permission: "usage.view" },
  },
  {
    path: "usage/budgets",
    lazy: lazyPage(() => import("./presentation/pages/BudgetsPage")),
    handle: { crumb: "Ngân sách", permission: "usage.view" },
  },
];

export const navItems: NavItem[] = [
  { to: "/usage", label: "Chi phí & ngân sách", icon: Wallet, group: "system", order: 10, permission: "usage.view" },
];

export const repositories: RepositoryFactories = {
  budget: () => new HttpBudgetRepository(),
};
