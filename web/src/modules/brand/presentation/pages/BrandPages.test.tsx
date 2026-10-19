import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { server } from "@/app/mocks/server";
import { api, fail, mockSession, rest } from "@/shared/api/mock";
import { renderApp } from "@/test/render";
import { MOCK_BRAND_ID, resetBrandMocks } from "../../infrastructure/mocks";

afterEach(() => resetBrandMocks());

describe("Brand Studio", () => {
  it("lists brands and creates one (lead)", async () => {
    mockSession.signInAs("lead@acs.local");
    const { router } = renderApp("/brands");
    expect(await screen.findByRole("cell", { name: "Anh ngữ Mẫu" })).toBeInTheDocument();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Tạo brand" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByLabelText(/Tên brand/), "Brand Mới");
    await user.click(within(dialog).getByRole("button", { name: "Tạo brand" }));
    await waitFor(() => expect(router.state.location.pathname).toMatch(/^\/brands\/[0-9a-f-]+$/));
    expect(await screen.findByRole("heading", { name: "Brand Mới" })).toBeInTheDocument();
  });

  it("shows the error state with retry", async () => {
    mockSession.signInAs("lead@acs.local");
    server.use(rest.get(api("/brands"), () => fail(500, "INTERNAL", "boom")));
    renderApp("/brands");
    expect(await screen.findByText("Không tải được dữ liệu.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Thử lại" })).toBeInTheDocument();
  });

  it("shows the empty state", async () => {
    mockSession.signInAs("lead@acs.local");
    server.use(rest.get(api("/brands"), () => new Response(JSON.stringify({ data: [], meta: { request_id: "r", next_cursor: null } }), { headers: { "Content-Type": "application/json" } })));
    renderApp("/brands");
    expect(await screen.findByText("Chưa có brand nào.")).toBeInTheDocument();
  });

  it.each(["creator@acs.local", "reviewer@acs.local"])("is read-only for %s (ACS-021)", async (email) => {
    mockSession.signInAs(email);
    renderApp(`/brands/${MOCK_BRAND_ID}`);
    expect(await screen.findByText("Bạn chỉ có quyền xem hồ sơ brand.")).toBeInTheDocument();
    expect(screen.getByLabelText(/Tên brand/)).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Lưu phiên bản mới" })).not.toBeInTheDocument();
  });

  it("saves a new version and handles VERSION_CONFLICT (ACS-020)", async () => {
    mockSession.signInAs("lead@acs.local");
    renderApp(`/brands/${MOCK_BRAND_ID}`);
    const user = userEvent.setup();
    const desc = await screen.findByLabelText("Mô tả");
    await user.clear(desc);
    await user.type(desc, "Mô tả mới");
    await user.click(screen.getByRole("button", { name: "Lưu phiên bản mới" }));
    expect(await screen.findByText("Phiên bản 2")).toBeInTheDocument();

    server.use(
      rest.patch(api("/brands/:id"), () =>
        fail(409, "VERSION_CONFLICT", "conflict", [{ current_version_no: 3, expected_version_no: 2 }]),
      ),
    );
    await user.type(screen.getByLabelText("Mô tả"), " nữa");
    await user.click(screen.getByRole("button", { name: "Lưu phiên bản mới" }));
    expect(await screen.findByText("Brand vừa được người khác cập nhật")).toBeInTheDocument();
    expect(screen.getByText(/Phiên bản hiện tại là 3/)).toBeInTheDocument();
  });

  it("rejects invalid hex colors before saving", async () => {
    mockSession.signInAs("lead@acs.local");
    renderApp(`/brands/${MOCK_BRAND_ID}`);
    const user = userEvent.setup();
    const hex = await screen.findByLabelText("Mã hex 1");
    await user.clear(hex);
    await user.type(hex, "xanh");
    await user.click(screen.getByRole("button", { name: "Lưu phiên bản mới" }));
    expect(await screen.findByText("Mã màu phải dạng #RRGGBB.")).toBeInTheDocument();
  });

  it("flags expired offers in the product list (ACS-022)", async () => {
    mockSession.signInAs("reviewer@acs.local");
    renderApp(`/brands/${MOCK_BRAND_ID}`);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("tab", { name: "Sản phẩm" }));
    const table = await screen.findByRole("table", { name: "Danh sách sản phẩm" });
    expect(await within(table).findByText("Khóa giao tiếp 12 tuần")).toBeInTheDocument();
    expect(within(table).getByText("1 hết hạn")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Thêm sản phẩm" })).not.toBeInTheDocument();
  });

  it("requires a license for music and uploads it (ACS-023)", async () => {
    mockSession.signInAs("lead@acs.local");
    renderApp(`/brands/${MOCK_BRAND_ID}`);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("tab", { name: "Tài nguyên" }));
    const form = await screen.findByRole("form", { name: "Tải lên tài nguyên" });
    await user.selectOptions(within(form).getByLabelText(/Loại tài nguyên/), "music");
    const file = new File(["id3"], "nhac.mp3", { type: "audio/mpeg" });
    await user.upload(within(form).getByLabelText(/^File/), file);
    await user.click(within(form).getByRole("button", { name: "Tải lên tài nguyên" }));
    expect(await within(form).findAllByText("Bắt buộc nhập.")).toHaveLength(2);

    await user.type(within(form).getByLabelText(/Nguồn nhạc/), "Artlist");
    await user.type(within(form).getByLabelText(/Loại giấy phép/), "commercial");
    await user.click(within(form).getByRole("button", { name: "Tải lên tài nguyên" }));
    const table = await screen.findByRole("table", { name: "Tài nguyên brand" });
    expect(await within(table).findByText("Hợp lệ")).toBeInTheDocument();
  });
});
