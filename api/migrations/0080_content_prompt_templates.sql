-- Generated from api/internal/modules/content/usecase/prompt/templates.go
-- (go test ./internal/modules/content/usecase/prompt -update). Do not edit by hand.
-- +goose Up
INSERT INTO prompt_templates (code, version, system_prompt, user_template, output_schema, status)
VALUES ('content.caption', 1,
$tpl$Bạn là copywriter marketing người Việt, viết nội dung cho thương hiệu được mô tả bên dưới. Định dạng: caption ngắn cho ảnh/video mạng xã hội — tối đa 3 câu, hook mạnh, CTA ngắn, 3–8 hashtag.$tpl$,
$tpl$Viết {{variants}} phương án caption cho {{platform}} loại {{content_type}}, mục tiêu {{objective}}, độ dài {{length}}, ngôn ngữ {{language}}.$tpl$,
$tpl${"type":"object","additionalProperties":false,"required":["title","hook","body","cta","hashtags"],"properties":{"title":{"type":"string"},"hook":{"type":"string"},"body":{"type":"string"},"cta":{"type":"string"},"hashtags":{"type":"array","items":{"type":"string"}}}}$tpl$::jsonb, 'active')
ON CONFLICT (code, version) DO NOTHING;

INSERT INTO prompt_templates (code, version, system_prompt, user_template, output_schema, status)
VALUES ('content.content_plan', 1,
$tpl$Bạn là chuyên gia lập kế hoạch nội dung marketing. Đề xuất content plan cho campaign: chủ đề, góc tiếp cận, lịch đăng (ngày YYYY-MM-DD trong thời gian campaign) và danh sách đầu ra khớp các định dạng được yêu cầu.$tpl$,
$tpl$Lập content plan cho campaign mục tiêu {{objective}}, ngôn ngữ {{language}}. Lịch đăng nằm trong khoảng {{start_date}} đến {{end_date}}.$tpl$,
$tpl${"type":"object","additionalProperties":false,"required":["summary","themes","schedule","deliverables"],"properties":{"summary":{"type":"string"},"themes":{"type":"array","minItems":2,"items":{"type":"object","additionalProperties":false,"required":["title","angle","key_message"],"properties":{"title":{"type":"string"},"angle":{"type":"string"},"key_message":{"type":"string"}}}},"schedule":{"type":"array","minItems":3,"items":{"type":"object","additionalProperties":false,"required":["date","channel","format","theme","idea"],"properties":{"date":{"type":"string"},"channel":{"type":"string"},"format":{"type":"string"},"theme":{"type":"string"},"idea":{"type":"string"}}}},"deliverables":{"type":"array","minItems":1,"items":{"type":"object","additionalProperties":false,"required":["format","count","notes"],"properties":{"format":{"type":"string"},"count":{"type":"integer"},"notes":{"type":"string"}}}}}}$tpl$::jsonb, 'active')
ON CONFLICT (code, version) DO NOTHING;

INSERT INTO prompt_templates (code, version, system_prompt, user_template, output_schema, status)
VALUES ('content.email', 1,
$tpl$Bạn là copywriter marketing người Việt, viết nội dung cho thương hiệu được mô tả bên dưới. Định dạng: email marketing — title là tiêu đề email (dưới 60 ký tự), hook là dòng preheader, body là nội dung email có lời chào và các đoạn ngắn, CTA là nhãn nút. hashtags để trống.$tpl$,
$tpl$Viết {{variants}} phương án email loại {{content_type}}, mục tiêu {{objective}}, khung {{framework}}, độ dài {{length}}, ngôn ngữ {{language}}.$tpl$,
$tpl${"type":"object","additionalProperties":false,"required":["title","hook","body","cta","hashtags"],"properties":{"title":{"type":"string"},"hook":{"type":"string"},"body":{"type":"string"},"cta":{"type":"string"},"hashtags":{"type":"array","items":{"type":"string"}}}}$tpl$::jsonb, 'active')
ON CONFLICT (code, version) DO NOTHING;

INSERT INTO prompt_templates (code, version, system_prompt, user_template, output_schema, status)
VALUES ('content.facebook_post', 1,
$tpl$Bạn là copywriter marketing người Việt, viết nội dung cho thương hiệu được mô tả bên dưới. Định dạng: bài đăng Facebook — tiêu đề ngắn, câu mở đầu (hook) gây chú ý trong 1–2 dòng, thân bài chia đoạn ngắn dễ đọc, CTA rõ ràng, 3–6 hashtag.$tpl$,
$tpl$Viết {{variants}} phương án bài đăng {{platform}} loại {{content_type}}, mục tiêu {{objective}}, khung {{framework}}, độ dài {{length}}, ngôn ngữ {{language}}. Các phương án phải khác nhau về góc tiếp cận.$tpl$,
$tpl${"type":"object","additionalProperties":false,"required":["title","hook","body","cta","hashtags"],"properties":{"title":{"type":"string"},"hook":{"type":"string"},"body":{"type":"string"},"cta":{"type":"string"},"hashtags":{"type":"array","items":{"type":"string"}}}}$tpl$::jsonb, 'active')
ON CONFLICT (code, version) DO NOTHING;

INSERT INTO prompt_templates (code, version, system_prompt, user_template, output_schema, status)
VALUES ('content.tiktok_script', 1,
$tpl$Bạn là copywriter marketing người Việt, viết nội dung cho thương hiệu được mô tả bên dưới. Định dạng: kịch bản TikTok 15–60 giây — hook 3 giây đầu, body là lời thoại theo nhịp nhanh (mỗi dòng một câu nói), CTA cuối video, 3–5 hashtag.$tpl$,
$tpl$Viết {{variants}} phương án kịch bản TikTok loại {{content_type}}, mục tiêu {{objective}}, khung {{framework}}, độ dài {{length}}, ngôn ngữ {{language}}.$tpl$,
$tpl${"type":"object","additionalProperties":false,"required":["title","hook","body","cta","hashtags"],"properties":{"title":{"type":"string"},"hook":{"type":"string"},"body":{"type":"string"},"cta":{"type":"string"},"hashtags":{"type":"array","items":{"type":"string"}}}}$tpl$::jsonb, 'active')
ON CONFLICT (code, version) DO NOTHING;

INSERT INTO prompt_templates (code, version, system_prompt, user_template, output_schema, status)
VALUES ('content.video_script', 1,
$tpl$Bạn là copywriter marketing người Việt, viết nội dung cho thương hiệu được mô tả bên dưới. Định dạng: kịch bản video ngắn tối đa 60 giây, 3–10 cảnh, mỗi cảnh 2–8 giây. body là toàn bộ lời dẫn; scenes mô tả từng cảnh: narration (lời dẫn), visual_hint (mô tả hình ảnh, không chứa chữ trên hình), duration_sec.$tpl$,
$tpl$Viết {{variants}} phương án kịch bản video cho {{platform}} loại {{content_type}}, mục tiêu {{objective}}, độ dài {{length}}, ngôn ngữ {{language}}.$tpl$,
$tpl${"type":"object","additionalProperties":false,"required":["title","hook","body","cta","hashtags","scenes"],"properties":{"title":{"type":"string"},"hook":{"type":"string"},"body":{"type":"string"},"cta":{"type":"string"},"hashtags":{"type":"array","items":{"type":"string"}},"scenes":{"type":"array","minItems":3,"maxItems":10,"items":{"type":"object","additionalProperties":false,"required":["narration","visual_hint","duration_sec"],"properties":{"narration":{"type":"string"},"visual_hint":{"type":"string"},"duration_sec":{"type":"integer"}}}}}}$tpl$::jsonb, 'active')
ON CONFLICT (code, version) DO NOTHING;

INSERT INTO prompt_templates (code, version, system_prompt, user_template, output_schema, status)
VALUES ('image.prompt_suggest', 1,
$tpl$Bạn là art director. Đề xuất đúng 3 prompt sinh ảnh bằng tiếng Anh, mỗi prompt mô tả bố cục, chủ thể, ánh sáng, phong cách, và BẮT BUỘC dùng bảng màu thương hiệu (nêu tên màu và mã hex) cùng cảm xúc theo tone thương hiệu. Không yêu cầu chữ trong ảnh, không dùng logo hay nhân vật có bản quyền.$tpl$,
$tpl$Đề xuất 3 prompt ảnh cho preset {{preset}}. rationale giải thích ngắn bằng tiếng Việt vì sao prompt phù hợp brief.$tpl$,
$tpl${"type":"object","additionalProperties":false,"required":["prompts"],"properties":{"prompts":{"type":"array","minItems":3,"maxItems":3,"items":{"type":"object","additionalProperties":false,"required":["prompt","negative_prompt","rationale"],"properties":{"prompt":{"type":"string"},"negative_prompt":{"type":"string"},"rationale":{"type":"string"}}}}}}$tpl$::jsonb, 'active')
ON CONFLICT (code, version) DO NOTHING;

-- +goose Down
DELETE FROM prompt_templates
WHERE version = 1 AND code IN ('content.caption', 'content.content_plan', 'content.email', 'content.facebook_post', 'content.tiktok_script', 'content.video_script', 'image.prompt_suggest')
  AND id NOT IN (SELECT prompt_template_id FROM ai_jobs WHERE prompt_template_id IS NOT NULL);
