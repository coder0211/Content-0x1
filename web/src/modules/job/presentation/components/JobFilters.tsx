import type { JobStatus } from "@/shared/kernel";
import { Input } from "@/shared/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { statusLabel } from "@/shared/ui/patterns";
import { JOB_STATUSES, JOB_TYPE_LABELS, JOB_TYPES, type JobFilter, type JobType } from "../../domain/job";

const ALL = "all";

export interface JobFiltersProps {
  value: JobFilter;
  onChange: (next: JobFilter) => void;
  /** Users with provider.view see every job and may filter by creator. */
  canSeeAll: boolean;
  currentUserId: string | undefined;
}

/** Toolbar: status, type, provider, creator (SPEC-05 work-tool filters). */
export function JobFilters({ value, onChange, canSeeAll, currentUserId }: JobFiltersProps) {
  return (
    <div className="mb-4 flex flex-wrap items-center gap-2" role="search" aria-label="Lọc job">
      <Select
        value={value.status ?? ALL}
        onValueChange={(v) => onChange({ ...value, status: v === ALL ? undefined : (v as JobStatus) })}
      >
        <SelectTrigger className="w-44" aria-label="Trạng thái">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>Mọi trạng thái</SelectItem>
          {JOB_STATUSES.map((s) => (
            <SelectItem key={s} value={s}>
              {statusLabel("job", s)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select value={value.type ?? ALL} onValueChange={(v) => onChange({ ...value, type: v === ALL ? undefined : (v as JobType) })}>
        <SelectTrigger className="w-52" aria-label="Loại job">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>Mọi loại</SelectItem>
          {JOB_TYPES.map((t) => (
            <SelectItem key={t} value={t}>
              {JOB_TYPE_LABELS[t]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Input
        className="w-44"
        placeholder="Nhà cung cấp"
        aria-label="Nhà cung cấp"
        defaultValue={value.providerCode ?? ""}
        onKeyDown={(e) => {
          if (e.key === "Enter") onChange({ ...value, providerCode: e.currentTarget.value.trim() || undefined });
        }}
        onBlur={(e) => {
          const v = e.currentTarget.value.trim() || undefined;
          if (v !== value.providerCode) onChange({ ...value, providerCode: v });
        }}
      />
      {canSeeAll ? (
        <Select
          value={value.createdBy ? (value.createdBy === currentUserId ? "me" : value.createdBy) : ALL}
          onValueChange={(v) => onChange({ ...value, createdBy: v === ALL ? undefined : v === "me" ? currentUserId : v })}
        >
          <SelectTrigger className="w-40" aria-label="Người tạo">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Mọi người tạo</SelectItem>
            <SelectItem value="me">Của tôi</SelectItem>
            {value.createdBy && value.createdBy !== currentUserId ? (
              <SelectItem value={value.createdBy}>Người dùng {value.createdBy.slice(0, 8)}</SelectItem>
            ) : null}
          </SelectContent>
        </Select>
      ) : null}
    </div>
  );
}
