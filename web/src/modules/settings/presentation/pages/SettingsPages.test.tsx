import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { resetSettingsMocks } from "@/modules/settings/infrastructure/mocks";
import { mockSession } from "@/shared/api/mock";
import { renderApp } from "@/test/render";

afterEach(() => resetSettingsMocks());

describe("ProvidersPage (ACS-090)", () => {
  it("lists providers without secrets and shows pricing versions of a model", async () => {
    mockSession.signInAs("admin@acs.local");
    renderApp("/settings/providers");
    const user = userEvent.setup();
    const table = await screen.findByRole("table", { name: "Danh sách provider" });
    expect(await within(table).findByText("PROVIDER_VEO_API_KEY")).toBeInTheDocument();
    expect(within(table).getByText("Chưa cấu hình trên server")).toBeInTheDocument();

    const models = await screen.findByRole("region", { name: "Model của Anthropic Claude" });
    await user.click(await within(models).findByText("claude-sonnet-5-5"));
    expect(await screen.findByText("anthropic-2026-10")).toBeInTheDocument();
    expect(screen.getByText("anthropic-2026-07")).toBeInTheDocument();
    expect(screen.getByText("Đang áp dụng")).toBeInTheDocument();

    // A duplicate pricing version is rejected client-side (old versions are immutable).
    await user.click(screen.getByRole("button", { name: "Phiên bản giá mới" }));
    const form = screen.getByRole("form", { name: "Phiên bản giá mới" });
    await user.type(within(form).getByLabelText(/Mã phiên bản/), "anthropic-2026-10");
    await user.click(within(form).getByRole("button", { name: "Lưu bảng giá" }));
    expect(within(form).getByText(/Phiên bản này đã tồn tại/)).toBeInTheDocument();
  });

  it("is read-only for a marketing lead (provider.view only)", async () => {
    mockSession.signInAs("lead@acs.local");
    renderApp("/settings/providers");
    await screen.findByRole("table", { name: "Danh sách provider" });
    expect(screen.getByText(/Chế độ chỉ xem/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Thêm model" })).not.toBeInTheDocument();
  });
});

describe("AuditLogPage (ACS-094)", () => {
  it("filters by resource type and opens before/after detail", async () => {
    mockSession.signInAs("admin@acs.local");
    renderApp("/settings/audit");
    const user = userEvent.setup();
    const table = await screen.findByRole("table", { name: "Nhật ký hệ thống" });
    await within(table).findAllByText("asset.approve");

    await user.selectOptions(screen.getByLabelText("Loại đối tượng"), "budget");
    await waitFor(() => expect(within(table).queryByText("asset.approve")).not.toBeInTheDocument());
    const rows = await within(table).findAllByText("budget.update");
    await user.click(rows[0]!);
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("limit_amount")).toBeInTheDocument();
    expect(within(dialog).getByText(/"limit_amount": 1200/)).toBeInTheDocument();
  });
});
