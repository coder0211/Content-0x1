import { cn } from "@/shared/lib/cn";
import { previewFile, type AssetType, type AssetVersion } from "../../domain/asset";

const str = (v: unknown): string => (typeof v === "string" ? v : "");

/** Read-only rendering of one version: text for content, media preview + prompt for image/video. */
export function AssetVersionContent({
  type,
  title,
  version,
  className,
}: {
  type: AssetType;
  title: string;
  version: AssetVersion;
  className?: string;
}) {
  const c = version.content;
  const media = previewFile(version.files);

  if (type === "content") {
    const hashtags = Array.isArray(c.hashtags) ? c.hashtags.filter((h): h is string => typeof h === "string") : [];
    const warnings = Array.isArray(c.warnings)
      ? c.warnings
          .map((w: unknown) => (typeof w === "string" ? w : typeof w === "object" && w !== null && "text" in w ? str((w as { text: unknown }).text) : ""))
          .filter(Boolean)
      : [];
    return (
      <article className={cn("flex flex-col gap-3 text-editor text-neutral-900", className)}>
        {str(c.title) ? <h3 className="text-section">{str(c.title)}</h3> : null}
        {str(c.hook) ? <p className="font-medium">{str(c.hook)}</p> : null}
        {str(c.body_text) ? <p className="whitespace-pre-line">{str(c.body_text)}</p> : null}
        {str(c.cta) ? <p className="font-medium">{str(c.cta)}</p> : null}
        {hashtags.length ? <p className="text-accent-700">{hashtags.join(" ")}</p> : null}
        {warnings.length ? (
          <ul className="list-disc pl-5 text-small text-warning">
            {warnings.map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
        ) : null}
      </article>
    );
  }

  return (
    <div className={cn("flex flex-col gap-3", className)}>
      {media ? (
        media.kind === "video" ? (
          <div className="flex justify-center rounded bg-video-bg">
            <video src={media.url ?? undefined} controls className="max-h-[480px]" aria-label={title} />
          </div>
        ) : (
          <div className="flex justify-center rounded border border-neutral-200 bg-neutral-100">
            <img src={media.url ?? undefined} alt={title} className="max-h-[480px] object-contain" />
          </div>
        )
      ) : (
        <p className="text-body text-neutral-500">Phiên bản này chưa có tệp xem trước.</p>
      )}
      {str(c.prompt) ? (
        <div>
          <p className="text-caption font-normal text-neutral-500">Prompt</p>
          <p className="whitespace-pre-line text-body text-neutral-900">{str(c.prompt)}</p>
        </div>
      ) : null}
      {str(c.script) ? (
        <div>
          <p className="text-caption font-normal text-neutral-500">Kịch bản</p>
          <p className="whitespace-pre-line text-body text-neutral-900">{str(c.script)}</p>
        </div>
      ) : null}
    </div>
  );
}
