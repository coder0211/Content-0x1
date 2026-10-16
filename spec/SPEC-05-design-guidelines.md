# SPEC-05 — Design Guidelines (UI/UX)

Phong cách: **"công cụ làm việc"**, giống Linear, Figma, Notion hay Frame.io. Giao diện trung tính và gọn để nội dung (bài viết, ảnh, video) nổi bật nhất. Đây là công cụ nội bộ dùng cả ngày, không phải trang giới thiệu sản phẩm AI.

## 1. Nguyên tắc

| # | Nguyên tắc | Ý nghĩa |
|---|---|---|
| P1 | **Nội dung là nhân vật chính** | Khung giao diện dùng màu trung tính; chỉ ảnh, video, bài viết và nút hành động chính có màu |
| P2 | **Vào thẳng việc** | Mỗi màn hình mở ra là thấy việc cần làm, không có hero, banner chào mừng hay minh họa trang trí |
| P3 | **AI kín đáo, minh bạch** | Không dùng sparkle hay gradient "phép màu". Nút ghi rõ việc sẽ làm, hiện chi phí dự kiến và nguồn gốc (model, phiên bản) |
| P4 | **Trạng thái luôn rõ** | Mọi job dài đều có tiến trình bằng chữ, thời gian ước tính và cách xử lý khi lỗi |
| P5 | **Nhất quán hơn sáng tạo** | Mỗi loại thành phần chỉ có một kiểu dùng. Không tự chế biến thể mới khi đã có component |

### 1.1. Danh sách cấm (anti-"AI look")

| ❌ Không dùng | ✅ Dùng thay |
|---|---|
| Gradient tím–xanh, chữ gradient, nền mesh/blur | Nền phẳng, tông stone, một màu nhấn |
| Icon ✨ sparkle, robot, "magic wand" | Icon chỉ hành động: `pen-line`, `image`, `clapperboard`, `refresh-cw` |
| Nút "Generate with AI", "Let AI do the magic" | "Viết 3 phương án", "Tạo 4 ảnh", "Sản xuất video" |
| Glassmorphism, `backdrop-blur`, bóng đổ lớn | Viền 1px `--border`, bóng `--shadow-sm` chỉ cho popover/dialog |
| `rounded-2xl`/`rounded-3xl`, nút dạng viên thuốc | Góc bo 6px (control), 8px (card/dialog) |
| Card lồng card, lưới card cho dữ liệu dạng bảng | Bảng hoặc danh sách; card chỉ dùng cho asset có ảnh |
| Emoji trong giao diện | Icon Lucide hoặc chữ |
| Vòng xoay to giữa màn hình, chữ "AI đang suy nghĩ…" | Thanh tiến trình mảnh + "Đang sinh cảnh 3/5 · còn ~2 phút" |
| Hiệu ứng nhấp nháy, shimmer kéo dài, confetti | Chuyển động 150–200ms ở các chỗ đổi trạng thái |
| Dark mode neon | Nền sáng; chỉ khu xem video dùng nền tối |

## 2. Design tokens

### 2.1. Màu

**Neutral (stone):** dùng cho nền, chữ, viền.

| Token | Hex | Dùng cho |
|---|---|---|
| `neutral-0` | `#FFFFFF` | Bề mặt card, input, bảng |
| `neutral-25` | `#FCFBFA` | Nền app |
| `neutral-50` | `#FAFAF9` | Nền sidebar |
| `neutral-100` | `#F5F5F4` | Nền khu xem trước (canvas), hover hàng bảng |
| `neutral-200` | `#E7E5E4` | Viền mặc định |
| `neutral-300` | `#D6D3D1` | Viền input khi hover, divider đậm |
| `neutral-400` | `#A8A29E` | Placeholder, icon phụ |
| `neutral-500` | `#78716C` | Chữ phụ (contrast 4.8:1) |
| `neutral-700` | `#44403C` | Chữ thường trong bảng |
| `neutral-900` | `#1C1917` | Chữ chính, tiêu đề |

**Accent (màu nhấn duy nhất):** thay bằng màu thương hiệu công ty nếu có, miễn đạt contrast ≥ 4.5:1 trên nền trắng.

| Token | Hex | Dùng cho |
|---|---|---|
| `accent-50` | `#EEF3FE` | Nền hàng/mục đang chọn |
| `accent-100` | `#DCE6FD` | Nền badge "đang chạy" |
| `accent-600` | `#1F4FD8` | Nút primary, link, focus ring, tiến trình |
| `accent-700` | `#1A40B0` | Nút primary khi hover/pressed |

**Semantic:** chỉ dùng để báo trạng thái, không dùng để trang trí.

| Trạng thái | Chữ / icon | Nền | Viền |
|---|---|---|---|
| success (đã duyệt, thành công) | `#15803D` | `#F0FDF4` | `#BBF7D0` |
| warning (chờ duyệt, gần hết ngân sách) | `#B45309` | `#FFFBEB` | `#FDE68A` |
| danger (lỗi, vượt ngân sách, xóa) | `#B91C1C` | `#FEF2F2` | `#FECACA` |
| info / running | `#1F4FD8` | `#EEF3FE` | `#DCE6FD` |
| neutral (nháp, đã hủy) | `#57534E` | `#F5F5F4` | `#E7E5E4` |

**Khu xem video:** nền `#0C0A09`, chữ `#E7E5E4`. Chỉ áp dụng trong khung player và timeline.

### 2.2. Typography

Font: **Be Vietnam Pro** (ưu tiên vì hiển thị dấu tiếng Việt chuẩn), dự phòng `Inter, system-ui, sans-serif`. Chỉ dùng weight 400/500/600.
Số liệu, chi phí, thời lượng, ID: `font-variant-numeric: tabular-nums`. Code/prompt thô: `JetBrains Mono`.

| Token | Size / line-height | Weight | Dùng cho |
|---|---|---|---|
| `text-page` | 20 / 28 | 600 | Tiêu đề trang |
| `text-section` | 16 / 24 | 600 | Tiêu đề khối |
| `text-body` | 14 / 20 | 400 | Mặc định |
| `text-body-strong` | 14 / 20 | 500 | Nhãn, tên trong bảng |
| `text-small` | 13 / 18 | 400 | Bảng, metadata |
| `text-caption` | 12 / 16 | 500 | Badge, chú thích, nhãn trục |
| `text-editor` | 16 / 26 | 400 | Nội dung bài viết trong editor (đọc dài) |

Không dùng chữ > 24px ở bất kỳ đâu trong app.

### 2.3. Spacing, radius, shadow, motion

| Nhóm | Giá trị |
|---|---|
| Spacing scale | 4 · 8 · 12 · 16 · 20 · 24 · 32 · 48 (px) |
| Padding trang | 24px; card 16px; ô bảng 8×12px |
| Chiều cao control | 32px (mặc định), 28px (compact trong bảng/toolbar), 36px (form chính) |
| Radius | `--radius-sm` 4px (badge), `--radius` 6px (button, input), `--radius-lg` 8px (card, dialog, ảnh thumbnail) |
| Shadow | `--shadow-sm: 0 1px 2px rgb(28 25 23 / .06)` (popover, dropdown); `--shadow-md: 0 4px 12px rgb(28 25 23 / .08)` (dialog). Card **không** có bóng, chỉ có viền |
| Border | 1px `neutral-200`; focus ring 2px `accent-600` offset 1px |
| Motion | 150ms `ease-out` cho hover/expand; 200ms cho dialog/drawer; tôn trọng `prefers-reduced-motion` |

### 2.4. Icon
- **Lucide**, stroke 1.5, size 16 (mặc định) / 20 (header, empty state). Màu theo chữ (`currentColor`).
- Danh sách icon chuẩn: content `pen-line`, image `image`, video `clapperboard`, campaign `megaphone`, brand `swatch-book`, library `library`, review `check-check`, job `activity`, cost `wallet`, settings `settings-2`.

### 2.5. Cấu hình Tailwind / shadcn

```css
/* web/src/shared/ui/theme.css */
:root {
  --background: 30 20% 99%;      /* neutral-25 */
  --foreground: 24 10% 10%;      /* neutral-900 */
  --card: 0 0% 100%;
  --muted: 60 5% 96%;            /* neutral-100 */
  --muted-foreground: 25 5% 45%; /* neutral-500 */
  --border: 20 6% 90%;           /* neutral-200 */
  --input: 20 6% 90%;
  --primary: 224 75% 48%;        /* accent-600 */
  --primary-foreground: 0 0% 100%;
  --ring: 224 75% 48%;
  --destructive: 0 74% 42%;
  --radius: 0.375rem;
}
html { font-family: "Be Vietnam Pro", Inter, system-ui, sans-serif; font-size: 14px; }
```

```ts
// tailwind.config.ts (trích)
theme: {
  extend: {
    borderRadius: { sm: '4px', DEFAULT: '6px', lg: '8px' },
    fontSize: { page: ['20px', '28px'], section: ['16px', '24px'], body: ['14px', '20px'], small: ['13px', '18px'], caption: ['12px', '16px'] },
    boxShadow: { sm: '0 1px 2px rgb(28 25 23 / .06)', md: '0 4px 12px rgb(28 25 23 / .08)' },
  },
},
```
Lint: cấm class `bg-gradient-*`, `backdrop-blur*`, `rounded-2xl|3xl|full` (trừ avatar), `shadow-lg|xl|2xl` bằng ESLint plugin `tailwindcss/no-custom-classname` + rule tùy chỉnh (ACS-004).

## 3. Component rules

| Component | Quy tắc |
|---|---|
| **Button** | 4 biến thể: `primary` (accent, tối đa **1 nút/khu vực**), `secondary` (viền), `ghost` (toolbar), `danger`. Nhãn là động từ + đối tượng: "Lưu phiên bản", "Gửi duyệt". Nút tạo AI có thể kèm chip chi phí bên phải |
| **CostChip** | `≈ $0.42` (ước tính) hoặc `$0.40` (thực tế), chữ `text-caption` tabular, tooltip ghi nguồn: "Ước tính theo bảng giá v3" / "Provider báo" |
| **StatusBadge** | Map 1-1 với state machine (SPEC-03 §2). Chấm tròn 6px + chữ. Không đổi màu theo ngữ cảnh |
| **JobProgress** | Thanh 4px + dòng chữ "Đang sinh cảnh 3/5 · còn ~2 phút" + nút "Hủy". Khi lỗi: chữ đỏ, nêu nguyên nhân dễ hiểu và nút "Thử lại" |
| **AIProvenance** | Nhãn nhỏ dưới nội dung AI: "Bản nháp AI · {model} · brand v{n} · {thời gian}". Không có icon sparkle |
| **Table** | Header `text-caption` uppercase off, nền `neutral-50`; hàng cao 40px; hover `neutral-100`; cột số căn phải; hành động hàng hiện khi hover (menu `…`) |
| **Form** | Label ở trên, helper text `text-small neutral-500`, lỗi hiện ngay dưới field kèm mã lỗi ở field nếu có. Field bắt buộc: dấu `*` sau label |
| **Dialog** | Chỉ cho xác nhận và form ngắn (≤ 5 field). Form dài dùng trang hoặc drawer phải 480px |
| **Toast** | Góc dưới phải, tự đóng 5s (lỗi thì không tự đóng), có link "Xem" tới job/asset |
| **Empty state** | Icon 20px + 1 câu mô tả + 1 nút hành động. Không dùng minh họa |
| **Loading** | Skeleton đúng hình dạng nội dung trong ≤ 1s; quá 1s thì hiện text trạng thái |
| **Media card** | Thumbnail tỉ lệ thật trên nền `neutral-100`, tên, StatusBadge, CostChip; chọn bằng viền `accent-600` 2px |
| **Warning inline** (từ cấm, claim chưa xác thực) | Gạch chân lượn sóng màu warning trong editor + danh sách ở panel phải; click để nhảy tới vị trí |

## 4. Layout

### 4.1. App shell
```
┌──────────┬───────────────────────────────────────────────────────┐
│ Sidebar  │ Topbar 48px: breadcrumb · tìm kiếm (⌘K) · thông báo · user │
│ 232px    ├───────────────────────────────────────────────────────┤
│ (thu gọn │ Nội dung trang (padding 24px, max-width tùy trang)      │
│  56px)   │                                                       │
└──────────┴───────────────────────────────────────────────────────┘
```
- Sidebar nhóm: **Làm việc** (Dashboard, Campaigns, Content, Image, Video), **Quản lý** (Library, Review [badge số chờ], Brand), **Hệ thống** (Settings).
- Chỉ hỗ trợ desktop ≥ 1280px (NFR-11). Từ 1280–1439px, panel phải của studio thu thành drawer.

### 4.2. Studio 3 cột (Content / Image / Video)
```
┌─────────────────┬──────────────────────────────┬────────────────┐
│ Brief & tùy chọn│ Canvas / Editor / Preview    │ Inspector      │
│ 320px, cuộn     │ co giãn, nền neutral-100     │ 300px          │
│                 │                              │ - Phiên bản    │
│ [CostChip]      │                              │ - Cảnh báo     │
│ [Nút primary]   │                              │ - Nguồn gốc    │
└─────────────────┴──────────────────────────────┴────────────────┘
```

### 4.3. Từng màn hình

| Màn hình | Bố cục | Điểm nhấn |
|---|---|---|
| Dashboard | 4 số liệu một hàng (campaign đang chạy, job đang chạy/lỗi, chờ duyệt, chi phí tháng) + 2 cột: "Cần bạn xử lý" và "Job gần đây" | Không dùng biểu đồ trang trí; chỉ hiện chi phí khi user có `usage.view` |
| Campaigns | Bảng: tên, brand, mục tiêu, trạng thái, deadline, số asset theo loại, chi phí | Lọc theo trạng thái dạng tab |
| Campaign Detail | Header (tên, trạng thái, ngân sách còn lại) + tabs Overview/Content/Images/Videos | Overview hiện brief và content plan dạng checklist đầu ra |
| Content Studio | Studio 3 cột; giữa là tab biến thể A/B/C + editor rộng tối đa 720px | Toolbar viết lại nhanh: Rút gọn · Mở rộng · Đổi tone · Yêu cầu khác |
| Image Studio | Trái: prompt, preset, tỉ lệ, số ảnh, ảnh tham chiếu. Giữa: lưới kết quả theo đợt tạo (mới nhất ở trên) | Lightbox xem ảnh 100%, phím ←/→ |
| Video Studio | Bước 1 Storyboard: danh sách cảnh dạng bảng có thể kéo thả. Bước 2 Sản xuất: player nền tối ở trên, dải cảnh (thumbnail + trạng thái) và timeline đơn giản ở dưới | Stepper 3 bước: Kịch bản → Storyboard → Sản xuất. Dự toán luôn hiện cạnh nút "Sản xuất video" |
| Asset Library | Toolbar lọc + chuyển đổi Grid/List; grid cho ảnh/video, list cho content | Chọn nhiều để tải hàng loạt (chỉ asset đã duyệt) |
| Review Queue | 2 cột: danh sách bên trái (360px), chi tiết bên phải với diff so với phiên bản trước | Phím tắt: `A` duyệt, `R` yêu cầu sửa, `J/K` chuyển mục |
| Settings | Menu dọc bên trái + form/bảng | Mọi thay đổi nhạy cảm có dialog xác nhận và được ghi audit |

## 5. Trạng thái màn hình (bắt buộc cho mọi màn)

| Trạng thái | Cách hiển thị |
|---|---|
| Loading | Skeleton theo hình dạng; không chặn toàn trang |
| Empty | Một câu + một nút: "Chưa có campaign nào. **Tạo campaign**" |
| Error tải dữ liệu | Khối báo lỗi tại chỗ + "Thử lại" + mã `request_id` để báo IT |
| Không có quyền | "Bạn không có quyền xem mục này. Liên hệ Admin." Không ẩn route im lặng |
| Job đang chạy | JobProgress; cho phép rời trang, job vẫn chạy và có toast khi xong |
| Job lỗi | Nguyên nhân dễ hiểu (map từ `error_kind`) + hành động gợi ý |
| Vượt ngân sách | Banner warning tại khu vực tạo: "Dự toán $3.20 vượt hạn mức mỗi job $2.00. Giảm số cảnh hoặc xin Marketing Lead duyệt." |

Map `error_kind` sang câu hiển thị:

| error_kind | Câu hiển thị |
|---|---|
| `ContentPolicy` | "Nội dung yêu cầu bị nhà cung cấp từ chối. Hãy chỉnh lại mô tả." |
| `RateLimited` | "Hệ thống đang bận, sẽ tự thử lại." |
| `Transient` (hết retry) | "Không kết nối được nhà cung cấp. Bấm Thử lại." |
| `InvalidRequest` | "Tham số không hợp lệ: {chi tiết}." |
| `BUDGET_EXCEEDED` | "Đã hết ngân sách {tháng/campaign}." |

## 6. Microcopy

- Tiếng Việt, ngắn, xưng "bạn". Không dùng "AI sẽ làm phép màu", "Tuyệt vời!".
- Nút: **động từ + đối tượng**. Tránh "OK", "Submit".
- Số liệu: tiền VND `2.990.000 ₫`; chi phí AI `$0.42` (2 chữ số, < $0.01 thì hiện `< $0.01`); ngày `16/10/2026`; giờ `14:05`; thời gian tương đối dưới 24h: "5 phút trước".
- Thời lượng video `0:20`; kích thước file `12,4 MB`.
- Thuật ngữ thống nhất: *campaign*, *brief*, *asset*, *phiên bản*, *cảnh*, *duyệt*, *yêu cầu sửa*, *dự toán*, *chi phí thực tế*.

## 7. Accessibility

- Contrast chữ ≥ 4.5:1 (chữ lớn ≥ 3:1); không truyền đạt trạng thái chỉ bằng màu (luôn kèm chữ hoặc icon).
- Toàn bộ thao tác làm được bằng bàn phím; focus ring luôn hiện khi điều hướng bằng phím.
- Dialog có focus trap, đóng bằng `Esc`; component Radix (shadcn) đáp ứng sẵn.
- Video player có phụ đề bật/tắt; ảnh trong library có `alt` từ tiêu đề asset.

## 8. Quy trình thiết kế

1. Token và component nằm trong `web/src/shared/ui` (SPEC-06), là nguồn sự thật duy nhất. Nếu dùng Figma thì Figma đồng bộ theo token này.
2. Trước khi code màn mới: wireframe xám (không màu) theo layout §4, PO duyệt.
3. PR UI cần ảnh chụp màn hình các trạng thái ở §5. Reviewer kiểm tra thêm danh sách cấm ở §1.1.
4. Storybook (P1) cho các component trong `shared/ui`.
