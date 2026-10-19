import type { CostSource } from "../../domain/budget";

/**
 * Series colors per cost_source (tokens only; fixed order, follows the entity).
 * Validated with the dataviz palette checker: CVD/normal-vision separation pass;
 * "calculated" is intentionally neutral gray (no special status), and identity is
 * never color-alone (legend + table view).
 */
export const SOURCE_FILL: Record<CostSource, string> = {
  provider_reported: "fill-accent-600",
  calculated: "fill-neutral-400",
  estimated: "fill-warning",
};

export const SOURCE_SWATCH: Record<CostSource, string> = {
  provider_reported: "bg-accent-600",
  calculated: "bg-neutral-400",
  estimated: "bg-warning",
};
