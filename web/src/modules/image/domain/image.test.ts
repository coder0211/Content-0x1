import {
  canSubmitForReview,
  capabilityErrors,
  cssAspect,
  defaultOptions,
  progressLabel,
  validateOptions,
  validateReferenceFile,
  type ImageBatch,
} from "./image";

describe("image domain", () => {
  it("defaults the ratio from the preset", () => {
    expect(defaultOptions("poster").aspectRatio).toBe("4:5");
    expect(defaultOptions("thumbnail").aspectRatio).toBe("16:9");
    expect(defaultOptions().count).toBe(1);
  });

  it("validates prompt, count, references and target", () => {
    const o = { ...defaultOptions(), prompt: "  ", count: 5, referenceFileIds: ["1", "2", "3", "4", "5"] };
    expect(validateOptions(o, false)).toEqual({
      prompt: "Nhập mô tả ảnh.",
      count: "Số ảnh từ 1 đến 4.",
      reference_file_ids: "Tối đa 4 ảnh tham chiếu.",
      target: "Chọn campaign hoặc brand.",
    });
    expect(validateOptions({ ...defaultOptions(), prompt: "Poster" }, true)).toEqual({});
  });

  it("maps 422 capability details to field messages with the valid values", () => {
    expect(
      capabilityErrors([
        { field: "aspect_ratio", supported: ["1:1", "16:9", "9:16"] },
        { field: "count", supported: [1, 2] },
        { field: "reference_file_ids", issue: "reference_image_not_supported", supported: [] },
      ]),
    ).toEqual({
      aspect_ratio: "Model không hỗ trợ giá trị này. Giá trị hợp lệ: 1:1, 16:9, 9:16.",
      count: "Model không hỗ trợ giá trị này. Giá trị hợp lệ: 1, 2.",
      reference_file_ids: "Model hiện tại không hỗ trợ ảnh tham chiếu.",
    });
  });

  it("formats aspect ratios for CSS", () => {
    expect(cssAspect("4:5")).toBe("4 / 5");
    expect(cssAspect("bad")).toBe("1 / 1");
  });

  it("describes job progress with concrete step text", () => {
    const b = { count: 4, job: { status: "running", progress: 30 } } as ImageBatch;
    expect(progressLabel(b)).toBe("Đang sinh 4 ảnh");
    expect(progressLabel({ ...b, job: { ...b.job, progress: 80 } })).toBe("Đang lưu ảnh");
    expect(progressLabel({ ...b, job: { ...b.job, status: "queued" } })).toBe("Đang chờ xử lý");
  });

  it("checks review state and reference files", () => {
    expect(canSubmitForReview("draft")).toBe(true);
    expect(canSubmitForReview("pending_review")).toBe(false);
    expect(validateReferenceFile({ type: "image/svg+xml", size: 10 })).toBe("Chỉ nhận ảnh PNG, JPEG hoặc WebP.");
    expect(validateReferenceFile({ type: "image/png", size: 21 * 1024 * 1024 })).toBe("Ảnh tham chiếu tối đa 20 MB.");
    expect(validateReferenceFile({ type: "image/webp", size: 1000 })).toBeNull();
  });
});
