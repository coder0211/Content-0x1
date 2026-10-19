import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http as mswHttp } from "msw";
import { server } from "@/app/mocks/server";
import { api, fail, mockSession } from "@/shared/api/mock";
import { renderApp } from "@/test/render";

async function login(email: string, password = "matkhau123") {
  const user = userEvent.setup();
  await user.type(await screen.findByLabelText(/Email/), email);
  await user.type(screen.getByLabelText(/Mật khẩu/), password);
  await user.click(screen.getByRole("button", { name: "Đăng nhập" }));
  return user;
}

describe("auth", () => {
  it("redirects anonymous users to /login with next, then back after login", async () => {
    const { router } = renderApp("/campaigns");
    await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    expect(router.state.location.search).toContain("next=%2Fcampaigns");

    await login("lead@acs.local");
    await waitFor(() => expect(router.state.location.pathname).toBe("/campaigns"));
    expect(await screen.findByRole("heading", { name: "Campaign" })).toBeInTheDocument();
  });

  it("shows an inline error for wrong credentials", async () => {
    renderApp("/login");
    await login("lead@acs.local", "sai");
    expect(await screen.findByText("Email hoặc mật khẩu không đúng.")).toBeInTheDocument();
  });

  it("validates fields client-side", async () => {
    renderApp("/login");
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Đăng nhập" }));
    expect(screen.getByText("Nhập email.")).toBeInTheDocument();
    expect(screen.getByText("Nhập mật khẩu.")).toBeInTheDocument();
  });

  it("hides menu items by permission (creator has no usage.view)", async () => {
    mockSession.signInAs("creator@acs.local");
    renderApp("/");
    const nav = await screen.findByRole("complementary", { name: "Điều hướng chính" });
    expect(within(nav).getByRole("link", { name: /Nội dung/ })).toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: /Chi phí & ngân sách/ })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: /Duyệt/ })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: /Cài đặt/ })).not.toBeInTheDocument();
  });

  it("shows NoPermission when opening a forbidden route directly", async () => {
    mockSession.signInAs("creator@acs.local");
    renderApp("/usage");
    expect(await screen.findByText("Bạn không có quyền xem mục này. Liên hệ Admin.")).toBeInTheDocument();
  });

  it("logs out to /login", async () => {
    mockSession.signInAs("admin@acs.local");
    const { router } = renderApp("/");
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /Tài khoản/ }));
    await user.click(await screen.findByRole("menuitem", { name: "Đăng xuất" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    expect(router.state.location.search).toBe("");
  });

  it("redirects to login with reason=expired when any API call returns 401", async () => {
    mockSession.signInAs("admin@acs.local");
    const { router, queryClient } = renderApp("/");
    await screen.findByRole("complementary", { name: "Điều hướng chính" });
    server.use(mswHttp.get(api("/anything"), () => fail(401, "SESSION_EXPIRED")));
    const { http } = await import("@/shared/api/http");
    await http.get("/anything").catch(() => undefined);
    await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    expect(router.state.location.search).toContain("reason=expired");
    expect(await screen.findByText("Phiên đăng nhập đã hết hạn. Đăng nhập lại để tiếp tục.")).toBeInTheDocument();
    expect(queryClient.getQueryData(["auth", "me"])).toBeNull();
  });
});
