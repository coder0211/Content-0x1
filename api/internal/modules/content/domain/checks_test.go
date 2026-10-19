package domain

import (
	"reflect"
	"testing"
)

// ACS-036 AC: unit test with 10 sample outputs.
func TestRunChecksTenSamples(t *testing.T) {
	forbidden := []string{"cam kết 100%", "rẻ nhất", "Số 1"}
	facts := NewFacts(
		[]string{"Giảm 20% học phí đến 31/12", "Hoàn phí trong 7 ngày", "Học phí trọn khóa 2.990.000đ"},
		Claim{Kind: ClaimMoney, Value: 2990000, Currency: "VND"},
	)
	type w = Warning
	cases := []struct {
		name string
		text string
		want []Warning
	}{
		{"clean", "Tự tin giao tiếp sau 12 tuần cùng giáo viên bản ngữ.", nil},
		{"verified percent", "Ưu đãi giảm 20% học phí cho sinh viên.", nil},
		{"unverified percent", "Chỉ hôm nay giảm 50% toàn bộ khóa học!", []w{{WarningUnverifiedClaim, "giảm 50%"}}},
		{"verified price other format", "Học phí chỉ 2,99 triệu cho trọn khóa.", nil},
		{"unverified price", "Học phí chỉ 1.990.000đ.", []w{{WarningUnverifiedClaim, "chỉ 1.990.000đ"}}},
		{"unverified k price", "Đăng ký ngay chỉ 299k.", []w{{WarningUnverifiedClaim, "chỉ 299k"}}},
		{"forbidden phrase", "Trung tâm rẻ nhất Hà Nội.", []w{{WarningForbiddenWord, "rẻ nhất"}}},
		{"forbidden case-insensitive", "Chúng tôi CAM KẾT 100% đầu ra.", []w{{WarningForbiddenWord, "cam kết 100%"}, {WarningUnverifiedClaim, "KẾT 100%"}}},
		{"forbidden needs whole word", "Lớp số 10 khai giảng.", nil},
		{"usd + both", "Tặng voucher $50, rẻ nhất thị trường.", []w{{WarningForbiddenWord, "rẻ nhất"}, {WarningUnverifiedClaim, "voucher $50"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RunChecks(Variant{Body: tc.text}, forbidden, facts)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestExtractClaims(t *testing.T) {
	cs := ExtractClaims("Giá 2.990.000 VND, giảm 15,5% còn 1 tỷ; tặng 5 khóa")
	if len(cs) != 3 {
		t.Fatalf("%+v", cs)
	}
	if cs[0].Kind != ClaimPercent || cs[0].Value != 15.5 {
		t.Fatalf("%+v", cs[0])
	}
	if cs[1].Value != 2990000 || cs[2].Value != 1e9 {
		t.Fatalf("%+v", cs)
	}
}

func TestRenderHTMLEscapes(t *testing.T) {
	got := RenderHTML("Xin chào <b>bạn</b>\ndòng 2\n\n<script>x</script>")
	want := "<p>Xin chào &lt;b&gt;bạn&lt;/b&gt;<br>dòng 2</p><p>&lt;script&gt;x&lt;/script&gt;</p>"
	if got != want {
		t.Fatal(got)
	}
}

func TestNormalizeVariant(t *testing.T) {
	v := Variant{Title: " T ", Hashtags: []string{"hoc tieng anh", "#a", "#a", ""}, Scenes: []Scene{{}, {Narration: "x", DurationSec: -1}}}.Normalize()
	if v.Title != "T" || !reflect.DeepEqual(v.Hashtags, []string{"#hoctienganh", "#a"}) || len(v.Scenes) != 1 || v.Scenes[0].DurationSec != 0 {
		t.Fatalf("%+v", v)
	}
}
