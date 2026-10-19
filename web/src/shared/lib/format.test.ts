import {
  formatDate,
  formatDateTime,
  formatDuration,
  formatEta,
  formatFileSize,
  formatRelative,
  formatTime,
  formatUsd,
  formatVnd,
} from "./format";

describe("format", () => {
  it("formats USD AI costs", () => {
    expect(formatUsd(0.42)).toBe("$0.42");
    expect(formatUsd(0.004)).toBe("< $0.01");
    expect(formatUsd(0)).toBe("$0.00");
    expect(formatUsd(1000)).toBe("$1,000.00");
    expect(formatUsd(0.42, { estimate: true })).toBe("≈ $0.42");
  });

  it("formats VND", () => {
    expect(formatVnd(2990000)).toBe("2.990.000 ₫");
  });

  it("formats dates in vi-VN", () => {
    const d = new Date(2026, 9, 16, 14, 5);
    expect(formatDate(d)).toBe("16/10/2026");
    expect(formatTime(d)).toBe("14:05");
    expect(formatDateTime(d)).toBe("16/10/2026 14:05");
  });

  it("formats relative time under 24h", () => {
    const now = new Date(2026, 9, 16, 14, 5);
    expect(formatRelative(new Date(now.getTime() - 20_000), now)).toBe("Vừa xong");
    expect(formatRelative(new Date(now.getTime() - 5 * 60_000), now)).toBe("5 phút trước");
    expect(formatRelative(new Date(now.getTime() - 3 * 3600_000), now)).toBe("3 giờ trước");
    expect(formatRelative(new Date(2026, 9, 14), now)).toBe("14/10/2026");
  });

  it("formats durations, ETA and file size", () => {
    expect(formatDuration(20)).toBe("0:20");
    expect(formatDuration(65)).toBe("1:05");
    expect(formatDuration(3723)).toBe("1:02:03");
    expect(formatEta(120)).toBe("~2 phút");
    expect(formatEta(28)).toBe("~30 giây");
    expect(formatFileSize(13_002_342)).toBe("12,4 MB");
    expect(formatFileSize(512)).toBe("512 B");
  });
});
