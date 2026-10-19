import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TooltipProvider } from "../tooltip";
import { CostChip } from "./CostChip";
import { FormField } from "./FormField";
import { JobProgress } from "./JobProgress";
import { StatusBadge } from "./StatusBadge";

describe("patterns", () => {
  it("StatusBadge maps status to a fixed label", () => {
    render(<StatusBadge kind="asset" status="pending_review" />);
    expect(screen.getByText("Chờ duyệt")).toBeInTheDocument();
  });

  it("CostChip shows estimate prefix", () => {
    render(
      <TooltipProvider>
        <CostChip amount={0.42} estimate />
      </TooltipProvider>,
    );
    expect(screen.getByText("≈ $0.42")).toBeInTheDocument();
  });

  it("JobProgress shows step text, ETA and cancel", async () => {
    const onCancel = vi.fn();
    render(<JobProgress status="running" progress={60} label="Đang sinh cảnh 3/5" etaSeconds={120} onCancel={onCancel} />);
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "60");
    expect(screen.getByText(/Đang sinh cảnh 3\/5/)).toHaveTextContent("Đang sinh cảnh 3/5 · còn ~2 phút");
    await userEvent.click(screen.getByRole("button", { name: "Hủy" }));
    expect(onCancel).toHaveBeenCalled();
  });

  it("JobProgress explains failures by error_kind and offers retry", () => {
    render(<JobProgress status="failed" errorKind="ContentPolicy" onRetry={() => undefined} />);
    expect(screen.getByText("Nội dung yêu cầu bị nhà cung cấp từ chối. Hãy chỉnh lại mô tả.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Thử lại" })).toBeInTheDocument();
  });

  it("FormField wires label, required marker and error", () => {
    render(
      <FormField label="Email" required error="Nhập email.">
        <input />
      </FormField>,
    );
    const input = screen.getByLabelText(/Email/);
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAccessibleDescription("Nhập email.");
  });
});
