import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router";
import { Clapperboard } from "lucide-react";
import { isApiError, type SceneStatus } from "@/shared/kernel";
import { useCan, useSession } from "@/shared/lib/session";
import { Skeleton } from "@/shared/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { toast } from "@/shared/ui/toast";
import { EmptyState, ErrorState, JobProgress, PageHeader } from "@/shared/ui/patterns";
import { useStoryboardDrafts } from "../../application/useStoryboardDrafts";
import { useVideoProject } from "../../application/useVideo";
import { isJobActive, jobProgressLabel, stepLabel, type VideoProject } from "../../domain/video";
import { BriefPanel } from "../components/BriefPanel";
import { EstimatePanel } from "../components/EstimatePanel";
import { ProductionView } from "../components/ProductionView";
import { RenderPanel } from "../components/RenderPanel";
import { StoryboardEditor } from "../components/StoryboardEditor";
import { VideoStepper } from "../components/VideoStepper";

/** /videos/:assetId — storyboard editor + production (ACS-071, ACS-072). */
export default function VideoProjectPage() {
  const { assetId = "" } = useParams();
  const q = useVideoProject(assetId);

  if (q.isLoading) return <ProjectSkeleton />;
  if (q.error) {
    if (isApiError(q.error) && q.error.code === "NOT_FOUND") {
      return (
        <EmptyState
          icon={Clapperboard}
          message="Không tìm thấy video hoặc bạn không có quyền xem."
          action={
            <Link to="/videos" className="text-body text-accent-700 underline">
              Về Video Studio
            </Link>
          }
        />
      );
    }
    return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  }
  if (!q.data) return null;
  return <ProjectStudio project={q.data} />;
}

function ProjectSkeleton() {
  return (
    <div aria-label="Đang tải dự án video" className="flex flex-col gap-4">
      <Skeleton className="h-7 w-80" />
      <div className="grid grid-cols-[320px_1fr_300px] gap-4">
        <Skeleton className="h-64" />
        <div className="flex flex-col gap-2">
          <Skeleton className="h-6 w-full" />
          <Skeleton className="h-32 w-full" />
          <Skeleton className="h-32 w-full" />
        </div>
        <Skeleton className="h-48" />
      </div>
    </div>
  );
}

type Tab = "storyboard" | "production";

function defaultTab(p: VideoProject): Tab {
  return p.stage === "storyboarding" || p.stage === "editing" ? "storyboard" : "production";
}

function ProjectStudio({ project }: { project: VideoProject }) {
  const { user } = useSession();
  const canGenerate = useCan("generate.run");
  const isLead = !!user && (user.roles.includes("admin") || user.roles.includes("marketing_lead"));
  const canEdit = canGenerate && (isLead || project.createdBy === user?.id);
  const sb = useStoryboardDrafts(project);
  const job = project.currentJob;
  const active = isJobActive(job);
  const [tab, setTab] = useState<Tab>(() => defaultTab(project));

  // Follow the project when a job starts or the storyboard arrives.
  const prevStage = useRef(project.stage);
  useEffect(() => {
    if (prevStage.current !== project.stage) {
      if (project.stage === "producing") setTab("production");
      if (project.stage === "editing" && prevStage.current === "storyboarding") setTab("storyboard");
      prevStage.current = project.stage;
    }
  }, [project.stage]);

  // Toast when a running job finishes (the user may be elsewhere on the page).
  const prevJob = useRef(job ? `${job.id}:${job.status}` : "");
  useEffect(() => {
    const cur = job ? `${job.id}:${job.status}` : "";
    const [prevId, prevStatus] = prevJob.current.split(":");
    if (job && prevId === job.id && (prevStatus === "queued" || prevStatus === "running")) {
      if (job.status === "succeeded") {
        toast.success(job.jobType === "video_storyboard" ? "Storyboard đã sẵn sàng" : `Đã render video phiên bản ${project.versionNo}`);
      } else if (job.status === "failed") {
        toast.error(null, { fallbackMessage: job.jobType === "video_storyboard" ? "Tạo storyboard lỗi" : "Sản xuất video lỗi" });
      }
    }
    prevJob.current = cur;
  }, [job, project.versionNo]);

  const statusById = useMemo(() => {
    const m: Record<string, SceneStatus> = {};
    for (const s of project.scenes) m[s.id] = s.status;
    return m;
  }, [project.scenes]);

  const storyboarding = project.stage === "storyboarding" || (job?.jobType === "video_storyboard" && job.status === "failed" && project.scenes.length === 0);

  return (
    <>
      <PageHeader title={project.title} actions={<VideoStepper stage={project.stage} />} />
      <div className="grid min-h-[600px] grid-cols-[320px_minmax(0,1fr)_300px] overflow-hidden rounded border border-neutral-200">
        <aside aria-label="Thông số" className="border-r border-neutral-200 bg-neutral-0 p-4">
          <BriefPanel project={project} />
        </aside>

        <div className="bg-neutral-100 p-4">
          {storyboarding && job ? (
            <section aria-label="Tạo storyboard" className="flex flex-col gap-4">
              <JobProgress
                status={job.status}
                progress={job.progress}
                label={active ? jobProgressLabel(job) : undefined}
                errorKind={job.steps.find((s) => s.status === "failed")?.errorKind ?? null}
                errorDetail={(() => {
                  const f = job.steps.find((s) => s.status === "failed");
                  return f ? `${stepLabel(f.stepKey)}: ${f.errorMessage ?? ""}` : (job.errorMessage ?? undefined);
                })()}
              />
              {active ? (
                <div className="flex flex-col gap-2" aria-hidden>
                  <Skeleton className="h-24 w-full" />
                  <Skeleton className="h-24 w-full" />
                  <Skeleton className="h-24 w-full" />
                </div>
              ) : (
                <p className="text-small text-neutral-600">Không tạo được storyboard. Tạo video mới từ Video Studio.</p>
              )}
            </section>
          ) : (
            <Tabs value={tab} onValueChange={(v) => setTab(v as Tab)}>
              <TabsList>
                <TabsTrigger value="storyboard">Storyboard</TabsTrigger>
                <TabsTrigger value="production">Sản xuất</TabsTrigger>
              </TabsList>
              <TabsContent value="storyboard" className="pt-4">
                <StoryboardEditor sb={sb} limits={project.limits} statusById={statusById} readOnly={!canEdit} locked={active} />
              </TabsContent>
              <TabsContent value="production" className="pt-4">
                <ProductionView project={project} canEdit={canEdit} />
              </TabsContent>
            </Tabs>
          )}
        </div>

        <aside aria-label="Bảng điều khiển" className="border-l border-neutral-200 bg-neutral-0 p-4">
          {storyboarding ? (
            <p className="text-small text-neutral-600">Dự toán sản xuất xuất hiện khi storyboard sẵn sàng.</p>
          ) : tab === "storyboard" ? (
            canEdit ? (
              <EstimatePanel assetId={project.id} issues={sb.issues} dirty={sb.dirty} jobActive={active} hasScenes={sb.drafts.length > 0} />
            ) : (
              <p className="text-small text-neutral-600">Bạn chỉ có quyền xem video này.</p>
            )
          ) : (
            <RenderPanel project={project} canEdit={canEdit} />
          )}
        </aside>
      </div>
    </>
  );
}
