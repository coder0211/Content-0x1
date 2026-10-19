import { useState } from "react";
import { formatDate, formatUsd } from "@/shared/lib/format";
import { COST_SOURCE_LABELS, COST_SOURCES, sourceAmount, type GroupBy, type UsageRow } from "../../domain/budget";
import { SOURCE_FILL, SOURCE_SWATCH } from "./sourceColors";

const H = 160;
const GAP = 2;

function rowLabel(r: UsageRow, groupBy: GroupBy): string {
  if (groupBy === "day" && r.key) return formatDate(`${r.key}T00:00:00`);
  return r.label || "—";
}

/**
 * Stacked bars of spend per group, split by cost_source. Plain SVG, no chart lib.
 * Horizontal bars for categories, vertical columns for days. The table below is the accessible view.
 */
export function UsageBarChart({ rows, groupBy }: { rows: UsageRow[]; groupBy: GroupBy }) {
  const [hover, setHover] = useState<number | null>(null);
  const max = Math.max(0, ...rows.map((r) => r.total));
  if (rows.length === 0 || max <= 0) return null;
  const hovered = hover !== null ? rows[hover] : null;

  return (
    <figure className="flex flex-col gap-2" aria-label="Biểu đồ chi phí">
      <div className="flex items-center gap-4 text-small text-neutral-600">
        {COST_SOURCES.map((s) => (
          <span key={s} className="inline-flex items-center gap-1.5">
            <span aria-hidden className={`inline-block size-2.5 rounded-sm ${SOURCE_SWATCH[s]}`} />
            {COST_SOURCE_LABELS[s]}
          </span>
        ))}
        <span className="ml-auto tabular text-neutral-500" aria-live="polite">
          {hovered ? `${rowLabel(hovered, groupBy)}: ${formatUsd(hovered.total)}` : `Cao nhất ${formatUsd(max)}`}
        </span>
      </div>
      {groupBy === "day" ? <Columns rows={rows} max={max} onHover={setHover} hover={hover} groupBy={groupBy} /> : <Bars rows={rows.slice(0, 12)} max={max} onHover={setHover} hover={hover} groupBy={groupBy} />}
    </figure>
  );
}

interface PlotProps {
  rows: UsageRow[];
  max: number;
  hover: number | null;
  onHover: (i: number | null) => void;
  groupBy: GroupBy;
}

function segments(r: UsageRow, scale: number) {
  let offset = 0;
  return COST_SOURCES.map((s) => {
    const len = sourceAmount(r, s) * scale;
    const seg = { source: s, start: offset, len };
    offset += len;
    return seg;
  }).filter((s) => s.len > 0);
}

function Columns({ rows, max, hover, onHover, groupBy }: PlotProps) {
  const slot = 24;
  const width = rows.length * slot;
  const scale = (H - 4) / max;
  return (
    <svg role="img" aria-label="Chi phí theo ngày" viewBox={`0 0 ${width} ${H + 18}`} className="h-44 w-full" preserveAspectRatio="none">
      <line x1={0} x2={width} y1={H} y2={H} className="stroke-neutral-200" strokeWidth={1} />
      {rows.map((r, i) => {
        const x = i * slot + 4;
        const w = slot - 8;
        return (
          <g key={r.key ?? i} onMouseEnter={() => onHover(i)} onMouseLeave={() => onHover(null)} opacity={hover === null || hover === i ? 1 : 0.5}>
            <title>{`${rowLabel(r, groupBy)}: ${formatUsd(r.total)}`}</title>
            <rect x={i * slot} y={0} width={slot} height={H} fill="transparent" />
            {segments(r, scale).map((s) => (
              <rect key={s.source} x={x} width={w} y={H - s.start - s.len} height={Math.max(0, s.len - GAP / 2)} rx={1} className={SOURCE_FILL[s.source]} />
            ))}
            {i % Math.ceil(rows.length / 10) === 0 ? (
              <text x={x + w / 2} y={H + 13} textAnchor="middle" className="fill-neutral-500 text-caption">
                {r.key?.slice(8, 10)}
              </text>
            ) : null}
          </g>
        );
      })}
    </svg>
  );
}

function Bars({ rows, max, hover, onHover, groupBy }: PlotProps) {
  const rowH = 22;
  const labelW = 0;
  const width = 600;
  const scale = (width - labelW) / max;
  return (
    <div className="flex flex-col gap-1">
      {rows.map((r, i) => (
        <div
          key={r.key ?? `none-${i}`}
          className="grid grid-cols-[180px_1fr_80px] items-center gap-3"
          onMouseEnter={() => onHover(i)}
          onMouseLeave={() => onHover(null)}
        >
          <span className="truncate text-small text-neutral-700" title={rowLabel(r, groupBy)}>
            {rowLabel(r, groupBy)}
          </span>
          <svg role="img" aria-label={`${rowLabel(r, groupBy)}: ${formatUsd(r.total)}`} viewBox={`0 0 ${width} ${rowH}`} preserveAspectRatio="none" className="h-4 w-full" opacity={hover === null || hover === i ? 1 : 0.5}>
            <title>{`${rowLabel(r, groupBy)}: ${formatUsd(r.total)}`}</title>
            {segments(r, scale).map((s) => (
              <rect key={s.source} x={s.start} y={0} width={Math.max(0, s.len - GAP / 2)} height={rowH} rx={1} className={SOURCE_FILL[s.source]} />
            ))}
          </svg>
          <span className="text-right text-small tabular text-neutral-700">{formatUsd(r.total)}</span>
        </div>
      ))}
    </div>
  );
}
