import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { mockSession } from "@/shared/api/mock";
import { renderApp } from "@/test/render";
import { MOCK_CAMPAIGN_ID, completeImageMockJobs, resetImageMocks } from "../../infrastructure/mocks";

describe("Image Studio page (ACS-053, Flow B in mock mode)", () => {
  afterEach(() => resetImageMocks());

  it("asks for a campaign or brand when none is chosen", async () => {
    mockSession.signInAs("creator@acs.local");
    renderApp("/images");
    expect(await screen.findByText(/Chọn campaign hoặc brand ở cột bên trái/)).toBeInTheDocument();
  });

  it("generates images for a campaign, shows them in the gallery and opens the inspector", async () => {
    mockSession.signInAs("creator@acs.local");
    renderApp(`/images?campaign_id=${MOCK_CAMPAIGN_ID}`);
    const user = userEvent.setup();

    const form = await screen.findByRole("form", { name: "Tạo ảnh" });
    // Seed batch is visible in the gallery.
    const main = screen.getByRole("main", { name: "Kết quả" });
    await within(main).findAllByRole("button", { name: /^Ảnh 1 — / });

    await user.type(within(form).getByLabelText(/Mô tả ảnh/), "Banner ưu đãi học phí tháng 11");
    // Estimate is priced through the CostChip.
    await within(form).findByText(/\$/);
    await user.click(within(form).getByRole("button", { name: /^Tạo \d ảnh$/ }));
    expect(await screen.findByText(/Đã gửi yêu cầu tạo/)).toBeInTheDocument();

    completeImageMockJobs();
    await waitFor(
      () => expect(within(main).getAllByRole("button", { name: /Banner ưu đãi học phí/ }).length).toBeGreaterThan(0),
      { timeout: 5000 },
    );

    await user.click(within(main).getAllByRole("button", { name: /Banner ưu đãi học phí/ })[0]!);
    const inspector = screen.getByRole("complementary", { name: "Chi tiết ảnh" });
    expect(await within(inspector).findByRole("button", { name: /biến thể/ })).toBeInTheDocument();
    expect(within(inspector).getByRole("button", { name: "Chỉnh sửa ảnh" })).toBeDisabled();
  });

  it("validates an empty prompt client-side", async () => {
    mockSession.signInAs("creator@acs.local");
    renderApp(`/images?campaign_id=${MOCK_CAMPAIGN_ID}`);
    const user = userEvent.setup();
    const form = await screen.findByRole("form", { name: "Tạo ảnh" });
    await user.click(within(form).getByRole("button", { name: /^Tạo \d ảnh$/ }));
    expect(await within(form).findByText("Nhập mô tả ảnh.")).toBeInTheDocument();
  });

  it("hides the studio from roles without generate.run", async () => {
    mockSession.signInAs("reviewer@acs.local");
    renderApp(`/images?campaign_id=${MOCK_CAMPAIGN_ID}`);
    await waitFor(() => expect(screen.queryByRole("form", { name: "Tạo ảnh" })).not.toBeInTheDocument());
    expect(await screen.findByText(/quyền/)).toBeInTheDocument();
  });
});
