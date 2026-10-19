import { useEffect, useState } from "react";
import { Search, X } from "lucide-react";
import type { AssetStatus } from "@/shared/kernel";
import { cn } from "@/shared/lib/cn";
import { Button } from "@/shared/ui/button";
import { controlClass, Input } from "@/shared/ui/input";
import { statusLabel } from "@/shared/ui/patterns";
import {
  ASSET_STATUSES,
  ASSET_TYPE_LABELS,
  ASSET_TYPES,
  EMPTY_FILTERS,
  hasActiveFilters,
  toggleStatus,
  type AssetFilters,
  type AssetType,
  type CampaignOption,
} from "../../domain/asset";

const selectClass = cn(controlClass, "h-8 w-auto pr-8");

/** Toolbar: search, campaign, type, status chips, tag, date range. Changes apply immediately. */
export function LibraryFilters({
  value,
  onChange,
  campaigns,
}: {
  value: AssetFilters;
  onChange: (f: AssetFilters) => void;
  campaigns: CampaignOption[];
}) {
  const [q, setQ] = useState(value.q);
  const [tag, setTag] = useState(value.tag);
  useEffect(() => setQ(value.q), [value.q]);
  useEffect(() => setTag(value.tag), [value.tag]);

  const set = (patch: Partial<AssetFilters>) => onChange({ ...value, ...patch });

  return (
    <div className="flex flex-col gap-3" role="search" aria-label="Bộ lọc thư viện">
      <div className="flex flex-wrap items-center gap-2">
        <form
          className="relative w-72"
          onSubmit={(e) => {
            e.preventDefault();
            set({ q });
          }}
        >
          <Search className="pointer-events-none absolute left-2 top-2 size-4 text-neutral-400" aria-hidden />
          <Input
            aria-label="Tìm kiếm"
            placeholder="Tìm theo tiêu đề, nội dung…"
            className="pl-8"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onBlur={() => q !== value.q && set({ q })}
          />
        </form>
        <select
          aria-label="Campaign"
          className={selectClass}
          value={value.campaignId ?? ""}
          onChange={(e) => set({ campaignId: e.target.value || null })}
        >
          <option value="">Tất cả campaign</option>
          {campaigns.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
        <select
          aria-label="Loại asset"
          className={selectClass}
          value={value.type ?? ""}
          onChange={(e) => set({ type: (e.target.value || null) as AssetType | null })}
        >
          <option value="">Tất cả loại</option>
          {ASSET_TYPES.map((t) => (
            <option key={t} value={t}>
              {ASSET_TYPE_LABELS[t]}
            </option>
          ))}
        </select>
        <Input
          aria-label="Tag"
          placeholder="Tag"
          className="w-32"
          value={tag}
          onChange={(e) => setTag(e.target.value)}
          onBlur={() => tag !== value.tag && set({ tag })}
          onKeyDown={(e) => {
            if (e.key === "Enter") set({ tag });
          }}
        />
        <Input aria-label="Từ ngày" type="date" className="w-36" value={value.from} onChange={(e) => set({ from: e.target.value })} />
        <Input aria-label="Đến ngày" type="date" className="w-36" value={value.to} onChange={(e) => set({ to: e.target.value })} />
        {hasActiveFilters(value) ? (
          <Button variant="ghost" size="sm" onClick={() => onChange(EMPTY_FILTERS)}>
            <X aria-hidden />
            Xóa bộ lọc
          </Button>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label="Trạng thái">
        <span className="mr-1 text-small text-neutral-500">Trạng thái:</span>
        {ASSET_STATUSES.map((s: AssetStatus) => {
          const active = value.statuses.includes(s);
          return (
            <button
              key={s}
              type="button"
              aria-pressed={active}
              onClick={() => set({ statuses: toggleStatus(value.statuses, s) })}
              className={cn(
                "h-7 rounded-sm border px-2 text-small transition-colors duration-150",
                active ? "border-accent-600 bg-accent-50 text-accent-700" : "border-neutral-200 bg-neutral-0 text-neutral-700 hover:bg-neutral-50",
              )}
            >
              {statusLabel("asset", s)}
            </button>
          );
        })}
      </div>
    </div>
  );
}
