package prompt

import "encoding/json"

// Template codes (prompt_templates.code). Version 1 of each is seeded by
// migration 0080 and mirrored here as a fallback when the provider module
// has no active row (e.g. placeholder/in-memory mode). A test keeps the
// migration and these defaults identical.
const (
	CodeFacebookPost       = "content.facebook_post"
	CodeTikTokScript       = "content.tiktok_script"
	CodeCaption            = "content.caption"
	CodeEmail              = "content.email"
	CodeVideoScript        = "content.video_script"
	CodeContentPlan        = "content.content_plan"
	CodeImagePromptSuggest = "image.prompt_suggest"
)

// CodeForFormat maps a content format (asset subtype) to its template code.
func CodeForFormat(format string) string { return "content." + format }

const contentItemSchema = `{"type":"object","additionalProperties":false,"required":["title","hook","body","cta","hashtags"],"properties":{"title":{"type":"string"},"hook":{"type":"string"},"body":{"type":"string"},"cta":{"type":"string"},"hashtags":{"type":"array","items":{"type":"string"}}}}`

const videoItemSchema = `{"type":"object","additionalProperties":false,"required":["title","hook","body","cta","hashtags","scenes"],"properties":{"title":{"type":"string"},"hook":{"type":"string"},"body":{"type":"string"},"cta":{"type":"string"},"hashtags":{"type":"array","items":{"type":"string"}},"scenes":{"type":"array","minItems":3,"maxItems":10,"items":{"type":"object","additionalProperties":false,"required":["narration","visual_hint","duration_sec"],"properties":{"narration":{"type":"string"},"visual_hint":{"type":"string"},"duration_sec":{"type":"integer"}}}}}}`

const planSchema = `{"type":"object","additionalProperties":false,"required":["summary","themes","schedule","deliverables"],"properties":{"summary":{"type":"string"},"themes":{"type":"array","minItems":2,"items":{"type":"object","additionalProperties":false,"required":["title","angle","key_message"],"properties":{"title":{"type":"string"},"angle":{"type":"string"},"key_message":{"type":"string"}}}},"schedule":{"type":"array","minItems":3,"items":{"type":"object","additionalProperties":false,"required":["date","channel","format","theme","idea"],"properties":{"date":{"type":"string"},"channel":{"type":"string"},"format":{"type":"string"},"theme":{"type":"string"},"idea":{"type":"string"}}}},"deliverables":{"type":"array","minItems":1,"items":{"type":"object","additionalProperties":false,"required":["format","count","notes"],"properties":{"format":{"type":"string"},"count":{"type":"integer"},"notes":{"type":"string"}}}}}}`

const imagePromptSchema = `{"type":"object","additionalProperties":false,"required":["prompts"],"properties":{"prompts":{"type":"array","minItems":3,"maxItems":3,"items":{"type":"object","additionalProperties":false,"required":["prompt","negative_prompt","rationale"],"properties":{"prompt":{"type":"string"},"negative_prompt":{"type":"string"},"rationale":{"type":"string"}}}}}}`

const writerSystem = "Bạn là copywriter marketing người Việt, viết nội dung cho thương hiệu được mô tả bên dưới."

// Defaults are version 1 of every template (seeded by migration 0080).
var Defaults = map[string]Template{
	CodeFacebookPost: {
		Code: CodeFacebookPost, Version: 1,
		System: writerSystem + " Định dạng: bài đăng Facebook — tiêu đề ngắn, câu mở đầu (hook) gây chú ý trong 1–2 dòng, thân bài chia đoạn ngắn dễ đọc, CTA rõ ràng, 3–6 hashtag.",
		User:   "Viết {{variants}} phương án bài đăng {{platform}} loại {{content_type}}, mục tiêu {{objective}}, khung {{framework}}, độ dài {{length}}, ngôn ngữ {{language}}. Các phương án phải khác nhau về góc tiếp cận.",
		Schema: json.RawMessage(contentItemSchema),
	},
	CodeTikTokScript: {
		Code: CodeTikTokScript, Version: 1,
		System: writerSystem + " Định dạng: kịch bản TikTok 15–60 giây — hook 3 giây đầu, body là lời thoại theo nhịp nhanh (mỗi dòng một câu nói), CTA cuối video, 3–5 hashtag.",
		User:   "Viết {{variants}} phương án kịch bản TikTok loại {{content_type}}, mục tiêu {{objective}}, khung {{framework}}, độ dài {{length}}, ngôn ngữ {{language}}.",
		Schema: json.RawMessage(contentItemSchema),
	},
	CodeCaption: {
		Code: CodeCaption, Version: 1,
		System: writerSystem + " Định dạng: caption ngắn cho ảnh/video mạng xã hội — tối đa 3 câu, hook mạnh, CTA ngắn, 3–8 hashtag.",
		User:   "Viết {{variants}} phương án caption cho {{platform}} loại {{content_type}}, mục tiêu {{objective}}, độ dài {{length}}, ngôn ngữ {{language}}.",
		Schema: json.RawMessage(contentItemSchema),
	},
	CodeEmail: {
		Code: CodeEmail, Version: 1,
		System: writerSystem + " Định dạng: email marketing — title là tiêu đề email (dưới 60 ký tự), hook là dòng preheader, body là nội dung email có lời chào và các đoạn ngắn, CTA là nhãn nút. hashtags để trống.",
		User:   "Viết {{variants}} phương án email loại {{content_type}}, mục tiêu {{objective}}, khung {{framework}}, độ dài {{length}}, ngôn ngữ {{language}}.",
		Schema: json.RawMessage(contentItemSchema),
	},
	CodeVideoScript: {
		Code: CodeVideoScript, Version: 1,
		System: writerSystem + " Định dạng: kịch bản video ngắn tối đa 60 giây, 3–10 cảnh, mỗi cảnh 2–8 giây. body là toàn bộ lời dẫn; scenes mô tả từng cảnh: narration (lời dẫn), visual_hint (mô tả hình ảnh, không chứa chữ trên hình), duration_sec.",
		User:   "Viết {{variants}} phương án kịch bản video cho {{platform}} loại {{content_type}}, mục tiêu {{objective}}, độ dài {{length}}, ngôn ngữ {{language}}.",
		Schema: json.RawMessage(videoItemSchema),
	},
	CodeContentPlan: {
		Code: CodeContentPlan, Version: 1,
		System: "Bạn là chuyên gia lập kế hoạch nội dung marketing. Đề xuất content plan cho campaign: chủ đề, góc tiếp cận, lịch đăng (ngày YYYY-MM-DD trong thời gian campaign) và danh sách đầu ra khớp các định dạng được yêu cầu.",
		User:   "Lập content plan cho campaign mục tiêu {{objective}}, ngôn ngữ {{language}}. Lịch đăng nằm trong khoảng {{start_date}} đến {{end_date}}.",
		Schema: json.RawMessage(planSchema),
	},
	CodeImagePromptSuggest: {
		Code: CodeImagePromptSuggest, Version: 1,
		System: "Bạn là art director. Đề xuất đúng 3 prompt sinh ảnh bằng tiếng Anh, mỗi prompt mô tả bố cục, chủ thể, ánh sáng, phong cách, và BẮT BUỘC dùng bảng màu thương hiệu (nêu tên màu và mã hex) cùng cảm xúc theo tone thương hiệu. Không yêu cầu chữ trong ảnh, không dùng logo hay nhân vật có bản quyền.",
		User:   "Đề xuất 3 prompt ảnh cho preset {{preset}}. rationale giải thích ngắn bằng tiếng Việt vì sao prompt phù hợp brief.",
		Schema: json.RawMessage(imagePromptSchema),
	},
}
