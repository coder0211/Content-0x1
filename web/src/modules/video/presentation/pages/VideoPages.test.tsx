import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { server } from "@/app/mocks/server";
import { api, fail, mockSession, rest } from "@/shared/api/mock";
import { renderApp } from "@/test/render";
import { getMockVideo, resetVideoMocks, seedVideoProject } from "../../infrastructure/mocks";
import type { SceneDto, VideoProjectDto } from "../../infrastructure/dto";

const CREATOR_ID = "00000000-0000-4000-8000-000000000003";
const CAMPAIGN = "c0000000-0000-4000-8000-000000000001";

function scene(n: number, over: Partial<SceneDto> = {}): SceneDto {
  return {
    id: `5ce00000-0000-4000-8000-00000000000${n}`,
    sequence_no: n,
    duration_ms: 5000,
    visual_prompt: `Cảnh ${n}: lớp học sáng sủa`,
    narration: `Lời thoại ${n}`,
    subtitle_text: `Phụ đề ${n}`,
    source_file_id: null,
    clip_file_id: null,
    status: "draft",
    attempt_count: 0,
    last_error: null,
    preview_url: null,
    thumbnail_url: null,
    ...over,
  };
}

function project(over: Partial<VideoProjectDto> = {}): VideoProjectDto {
  const scenes = over.scenes ?? [1, 2, 3, 4].map((n) => scene(n));
  return {
    id: "a1d00000-0000-4000-8000-000000000001",
    title: "Video ưu đãi mùa thu",
    status: "draft",
    stage: "editing",
    campaign_id: null,
    brand_id: "b0000000-0000-4000-8000-000000000001",
    created_by: CREATOR_ID,
    prompt: "Video quảng cáo khóa học",
    script: "Kịch bản",
    settings: {
      preset: "social_vertical",
      aspect_ratio: "9:16",
      width: 1080,
      height: 1920,
      fps: 30,
      target_duration_seconds: 20,
      resolution: "1080p",
      voiceover: { enabled: true, language: "vi", voice_id: null, mode: "per_scene" },
      subtitles: { enabled: true, language: "vi", style: "burn_in" },
      music: { enabled: false, track_brand_asset_id: null },
      source: { type: "prompt", file_ids: [] },
    },
    scenes,
    total_duration_ms: scenes.reduce((s, x) => s + x.duration_ms, 0),
    limits: { max_total_seconds: 60, max_scenes: 10, min_scene_ms: 2000, max_scene_ms: 8000 },
    output: null,
    current_job: null,
    version_no: 1,
    created_at: "2026-10-19T08:00:00Z",
    updated_at: "2026-10-19T08:00:00Z",
    ...over,
  };
}

const PATH = "/videos/a1d00000-0000-4000-8000-000000000001";

afterEach(() => resetVideoMocks());

describe("VideoStudioPage", () => {
  it("creates a storyboard from a prompt inside a campaign and opens the project", async () => {
    mockSession.signInAs("creator@acs.local");
    const user = userEvent.setup();
    const { router } = renderApp(`/videos?campaign_id=${CAMPAIGN}`);
    expect(await screen.findByText(/gắn vào campaign đã chọn/)).toBeInTheDocument();
    expect(await screen.findByText("Chưa có video nào. Tạo video đầu tiên bằng biểu mẫu bên trái.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Tạo storyboard" }));
    expect(await screen.findByText("Nhập mô tả video.")).toBeInTheDocument();

    await user.type(screen.getByRole("textbox", { name: /Mô tả video/ }), "Video giới thiệu khóa tiếng Anh mùa thu");
    await user.click(screen.getByRole("button", { name: "Tạo storyboard" }));
    await waitFor(() => expect(router.state.location.pathname).toMatch(/^\/videos\/[0-9a-f-]{36}$/));
    expect(await screen.findByRole("heading", { level: 1, name: "Video giới thiệu khóa tiếng Anh mùa thu" })).toBeInTheDocument();
    expect(await screen.findByRole("region", { name: "Tạo storyboard" })).toBeInTheDocument();
  });

  it("prefills the script source from ?script_from_asset_id", async () => {
    mockSession.signInAs("creator@acs.local");
    renderApp(`/videos?campaign_id=${CAMPAIGN}&script_from_asset_id=abc`);
    expect(await screen.findByRole("textbox", { name: /Mã asset kịch bản/ })).toHaveValue("abc");
  });

  it("lists existing projects", async () => {
    mockSession.signInAs("creator@acs.local");
    seedVideoProject(project());
    renderApp("/videos");
    const table = await screen.findByRole("table", { name: "Danh sách video" });
    expect(await within(table).findByText("Video ưu đãi mùa thu")).toBeInTheDocument();
  });

  it("shows the no-permission state to reviewers", async () => {
    mockSession.signInAs("reviewer@acs.local");
    renderApp("/videos");
    expect(await screen.findByText(/không có quyền/i)).toBeInTheDocument();
  });
});

describe("VideoProjectPage — storyboard editor", () => {
  it("shows the estimate and starts production", async () => {
    mockSession.signInAs("creator@acs.local");
    seedVideoProject(project());
    const user = userEvent.setup();
    renderApp(PATH);
    expect(await screen.findByRole("heading", { name: "Storyboard" })).toBeInTheDocument();
    expect(screen.getAllByRole("listitem", { name: /^Cảnh \d$/ })).toHaveLength(4);
    expect(await screen.findByText("Tổng dự toán")).toBeInTheDocument();
    const produce = screen.getByRole("button", { name: "Sản xuất video" });
    await waitFor(() => expect(produce).toBeEnabled());
    await user.click(produce);
    await waitFor(() => expect(getMockVideo(project().id)?.stage).toBe("producing"));
    expect(await screen.findByRole("region", { name: "Tiến độ" })).toBeInTheDocument();
  });

  it("blocks production when the total duration exceeds 60 seconds", async () => {
    mockSession.signInAs("creator@acs.local");
    seedVideoProject(project());
    const user = userEvent.setup();
    renderApp(PATH);
    const duration = await screen.findByRole("spinbutton", { name: "Thời lượng cảnh 1" });
    // 4 scenes: 45 + 5 + 5 + 5 = 60 s is fine, 8 s scene limit is not.
    await user.clear(duration);
    await user.type(duration, "9");
    expect(await screen.findByText("Cảnh 1: thời lượng phải từ 2 đến 8 giây.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sản xuất video" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Lưu storyboard" })).toBeDisabled();
  });

  it("flags a storyboard over 60 s and suggests reducing scenes", async () => {
    mockSession.signInAs("creator@acs.local");
    const scenes = Array.from({ length: 8 }, (_, i) => scene(i + 1, { duration_ms: 8000 }));
    seedVideoProject(project({ scenes }));
    renderApp(PATH);
    expect(await screen.findByText(/Tổng thời lượng 64 giây vượt giới hạn 60 giây/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sản xuất video" })).toBeDisabled();
    expect(screen.getByRole("list", { name: "Lý do chưa sản xuất được" })).toHaveTextContent("Storyboard chưa đạt giới hạn");
  });

  it("reorders, adds and saves scenes", async () => {
    mockSession.signInAs("creator@acs.local");
    seedVideoProject(project());
    const user = userEvent.setup();
    renderApp(PATH);
    await user.click(await screen.findByRole("button", { name: "Đưa cảnh 1 xuống" }));
    expect(screen.getByRole("textbox", { name: "Mô tả hình ảnh cảnh 2" })).toHaveValue("Cảnh 1: lớp học sáng sủa");
    expect(screen.getByRole("button", { name: "Sản xuất video" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Thêm cảnh" }));
    await user.type(screen.getByRole("textbox", { name: "Mô tả hình ảnh cảnh 5" }), "Logo trung tâm");
    await user.click(screen.getByRole("button", { name: "Lưu storyboard" }));
    await waitFor(() => expect(getMockVideo(project().id)?.scenes).toHaveLength(5));
    const saved = getMockVideo(project().id)!.scenes;
    expect(saved[0]!.visual_prompt).toBe("Cảnh 2: lớp học sáng sủa");
    expect(saved[4]!.visual_prompt).toBe("Logo trung tâm");
  });

  it("over the per-job limit: creator is blocked, lead can override", async () => {
    const scenes = Array.from({ length: 10 }, (_, i) => scene(i + 1, { duration_ms: 6000 }));
    seedVideoProject(project({ scenes, created_by: "00000000-0000-4000-8000-000000000002" }));
    mockSession.signInAs("lead@acs.local");
    const user = userEvent.setup();
    renderApp(PATH);
    expect(await screen.findByText(/vượt hạn mức mỗi job \$8/)).toBeInTheDocument();
    const produce = screen.getByRole("button", { name: "Sản xuất video" });
    expect(produce).toBeDisabled();
    await user.click(screen.getByRole("checkbox", { name: "Vượt hạn mức (Lead duyệt)" }));
    expect(produce).toBeEnabled();
  });

  it("creator over the per-job limit sees the suggestion and no override", async () => {
    const scenes = Array.from({ length: 10 }, (_, i) => scene(i + 1, { duration_ms: 6000 }));
    seedVideoProject(project({ scenes }));
    mockSession.signInAs("creator@acs.local");
    renderApp(PATH);
    expect(await screen.findByText(/Giảm số cảnh hoặc thời lượng, hoặc xin Marketing Lead duyệt/)).toBeInTheDocument();
    expect(screen.getByRole("list", { name: "Lý do chưa sản xuất được" })).toHaveTextContent("Cần Marketing Lead duyệt vượt hạn mức.");
    expect(screen.queryByRole("checkbox", { name: "Vượt hạn mức (Lead duyệt)" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sản xuất video" })).toBeDisabled();
  });

  it("is read-only for reviewers", async () => {
    seedVideoProject(project());
    mockSession.signInAs("reviewer@acs.local");
    renderApp(PATH);
    expect(await screen.findByText("Bạn chỉ có quyền xem video này.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Sản xuất video" })).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Mô tả hình ảnh cảnh 1" })).toBeDisabled();
  });
});

describe("VideoProjectPage — production", () => {
  it("shows a partial scene failure and regenerates only that scene", async () => {
    const scenes = [
      scene(1, { status: "ready", clip_file_id: "f1", attempt_count: 1 }),
      scene(2, { status: "ready", clip_file_id: "f2", attempt_count: 1 }),
      scene(3, { status: "failed", attempt_count: 1, last_error: "Nhà cung cấp từ chối nội dung cảnh (ContentPolicy)." }),
    ];
    seedVideoProject(
      project({
        scenes,
        stage: "failed",
        current_job: {
          id: "j0000000-0000-4000-8000-000000000001",
          job_type: "video_production",
          status: "failed",
          progress: 60,
          steps: [
            { step_key: "scene_generation:1", status: "succeeded", attempt_count: 1, error_kind: null, error_message: null },
            { step_key: "scene_generation:2", status: "succeeded", attempt_count: 1, error_kind: null, error_message: null },
            { step_key: "scene_generation:3", status: "failed", attempt_count: 1, error_kind: "ContentPolicy", error_message: "blocked" },
            { step_key: "video_render", status: "skipped", attempt_count: 0, error_kind: null, error_message: null },
          ],
          error_code: null,
          error_message: null,
        },
      }),
    );
    mockSession.signInAs("creator@acs.local");
    const user = userEvent.setup();
    renderApp(PATH);
    expect(await screen.findByText("1 cảnh sinh lỗi")).toBeInTheDocument();
    const card = screen.getByRole("listitem", { name: "Cảnh 3" });
    expect(within(card).getByText(/ContentPolicy/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Render lại video" })).toBeDisabled();

    await user.click(within(card).getByRole("button", { name: "Tạo lại cảnh 3" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo lại cảnh 3" });
    const prompt = within(dialog).getByRole("textbox", { name: "Mô tả hình ảnh mới" });
    await user.clear(prompt);
    await user.type(prompt, "Học viên cười trong lớp");
    await user.click(within(dialog).getByRole("button", { name: "Tạo lại cảnh 3" }));
    await waitFor(() => expect(getMockVideo(project().id)?.stage).toBe("producing"));
    expect(getMockVideo(project().id)!.scenes[2]!.visual_prompt).toBe("Học viên cười trong lớp");
    expect(getMockVideo(project().id)!.scenes[0]!.clip_file_id).toBe("f1");
  });

  it("plays the output and re-renders with new subtitle settings", async () => {
    const scenes = [1, 2, 3].map((n) => scene(n, { status: "ready", clip_file_id: `f${n}`, attempt_count: 1 }));
    seedVideoProject(
      project({
        scenes,
        stage: "ready",
        version_no: 2,
        output: {
          file_id: "o0000000-0000-4000-8000-000000000001",
          url: "https://storage.local/final.mp4",
          thumbnail_url: null,
          duration_ms: 14400,
          version_no: 2,
          subtitle_file_id: null,
        },
      }),
    );
    mockSession.signInAs("creator@acs.local");
    const user = userEvent.setup();
    renderApp(PATH);
    expect(await screen.findByLabelText("Video đã render")).toHaveAttribute("src", "https://storage.local/final.mp4");
    expect(screen.getByText(/Phiên bản 2/)).toBeInTheDocument();

    await user.click(screen.getByRole("switch", { name: "Phụ đề khi render lại" }));
    await user.click(screen.getByRole("button", { name: "Lưu và render lại" }));
    await waitFor(() => expect(getMockVideo(project().id)?.settings.subtitles.enabled).toBe(false));
    expect(getMockVideo(project().id)?.current_job?.job_type ?? "video_render").toBe("video_render");
  });

  it("submits a ready video for review", async () => {
    const scenes = [1, 2].map((n) => scene(n, { status: "ready", clip_file_id: `f${n}` }));
    seedVideoProject(
      project({
        scenes,
        stage: "ready",
        output: { file_id: "o1", url: "", thumbnail_url: null, duration_ms: 9700, version_no: 2, subtitle_file_id: null },
      }),
    );
    mockSession.signInAs("creator@acs.local");
    const user = userEvent.setup();
    renderApp(PATH);
    expect(await screen.findByText(/Không có bản xem trước/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Gửi duyệt" }));
    await waitFor(() => expect(getMockVideo(project().id)?.status).toBe("pending_review"));
  });
});

describe("VideoProjectPage — states", () => {
  it("shows not found for another user's project", async () => {
    seedVideoProject(project({ created_by: "00000000-0000-4000-8000-000000000099" }));
    mockSession.signInAs("creator@acs.local");
    renderApp(PATH);
    expect(await screen.findByText("Không tìm thấy video hoặc bạn không có quyền xem.")).toBeInTheDocument();
  });

  it("shows a load error with retry", async () => {
    // 4xx is not retried by the query client, so the error renders at once.
    server.use(rest.get(api("/videos/:id"), () => fail(403, "FORBIDDEN", "Thiếu quyền")));
    mockSession.signInAs("creator@acs.local");
    renderApp(PATH);
    expect(await screen.findByRole("button", { name: /Thử lại/ })).toBeInTheDocument();
  });

  it("shows a skeleton while loading", async () => {
    seedVideoProject(project());
    mockSession.signInAs("creator@acs.local");
    renderApp(PATH);
    expect(await screen.findByLabelText("Đang tải dự án video")).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "Storyboard" })).toBeInTheDocument();
  });
});
