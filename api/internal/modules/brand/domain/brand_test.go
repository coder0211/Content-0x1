package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/shared/kernel"
)

func TestIsHexColor(t *testing.T) {
	for s, want := range map[string]bool{"#0055FF": true, "#fff": true, "#0055ff": true, "0055FF": false, "#0055FG": false, "#12345": false, "": false, "red": false} {
		if IsHexColor(s) != want {
			t.Errorf("%q: want %v", s, want)
		}
	}
}

func issues(err error) []string {
	de, _ := kernel.AsDomainError(err)
	if de == nil {
		return nil
	}
	var out []string
	for _, d := range de.Details {
		out = append(out, d["field"].(string)+":"+d["issue"].(string))
	}
	return out
}

func TestProfileValidate(t *testing.T) {
	p := Profile{
		Colors:           []Color{{Name: "primary", Hex: "#0055ff"}, {Name: "", Hex: "blue"}},
		PreferredWords:   []string{"rẻ nhất"},
		ForbiddenWords:   []string{" Rẻ nhất ", "rẻ nhất"},
		AudiencePersonas: []Persona{{Name: ""}},
	}.Normalize()
	if len(p.ForbiddenWords) != 1 || p.Colors[0].Hex != "#0055FF" || p.ToneOfVoice == nil {
		t.Fatalf("normalize: %+v", p)
	}
	var v kernel.ValidationErrors
	p.Validate(&v, "profile")
	got := strings.Join(issues(v.Err()), ",")
	for _, want := range []string{"profile.colors[1].name:required", "profile.colors[1].hex:invalid_hex",
		"profile.audience_personas[0].name:required", "profile.preferred_words:conflicts_with_forbidden_words"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, "colors[0]") {
		t.Errorf("valid color flagged: %s", got)
	}
}

func TestProfileEqual(t *testing.T) {
	a := Profile{ToneOfVoice: []string{"friendly"}}.Normalize()
	b := Profile{ToneOfVoice: []string{" friendly ", "Friendly"}}.Normalize()
	if !a.Equal(b) {
		t.Fatal("normalized profiles should be equal")
	}
	if a.Equal(Profile{ToneOfVoice: []string{"formal"}}.Normalize()) {
		t.Fatal("different profiles equal")
	}
}

func TestCheckVersion(t *testing.T) {
	b := &Brand{Current: Version{VersionNo: 3}}
	if err := b.CheckVersion(3); err != nil {
		t.Fatal(err)
	}
	if err := b.CheckVersion(2); kernel.ErrorCode(err) != kernel.CodeVersionConflict {
		t.Fatalf("got %v", err)
	}
	nv := b.NextVersion(Profile{}, kernel.NewID(), time.Now())
	if nv.VersionNo != 4 {
		t.Fatal(nv.VersionNo)
	}
}

func TestOfferStatusAndPromptFacts(t *testing.T) {
	// 2026-10-19 20:00 UTC is already 2026-10-20 in Vietnam.
	at := time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)
	f := Facts{
		Offers: []Offer{
			{Text: "expired", ValidTo: "2026-10-19"},
			{Text: "today", ValidFrom: "2026-10-20", ValidTo: "2026-10-20"},
			{Text: "open"},
			{Text: "upcoming", ValidFrom: "2026-11-01"},
		},
		Proof: []Proof{{Text: "ok", Verified: true}, {Text: "nope"}},
	}.Normalize()
	want := []string{OfferExpired, OfferActive, OfferActive, OfferUpcoming}
	for i, o := range f.Offers {
		if o.StatusAt(at) != want[i] {
			t.Errorf("%s: %s", o.Text, o.StatusAt(at))
		}
	}
	pf := f.PromptFacts(at)
	if len(pf.Offers) != 2 || pf.Offers[0].Text != "today" || len(pf.Proof) != 1 {
		t.Fatalf("%+v", pf)
	}
}

func TestFactsValidateAndPriceJSON(t *testing.T) {
	f := Facts{
		Price:  &Price{Amount: decimal.NewFromInt(-1), Currency: "vn"},
		Offers: []Offer{{Text: "", ValidFrom: "2026-12-01", ValidTo: "2026-11-01"}, {Text: "x", ValidTo: "31/12"}},
		Proof:  []Proof{{Type: "rumor", Text: ""}},
	}.Normalize()
	var v kernel.ValidationErrors
	f.Validate(&v, "facts")
	got := strings.Join(issues(v.Err()), ",")
	for _, want := range []string{"facts.price.amount:negative", "facts.price.currency:invalid", "facts.offers[0].text:required",
		"facts.offers[0].valid_to:before_valid_from", "facts.offers[1].valid_to:invalid_date", "facts.proof[0].type:invalid", "facts.proof[0].text:required"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	b, _ := json.Marshal(Price{Amount: decimal.NewFromInt(2990000), Currency: "VND"})
	if !strings.Contains(string(b), `"amount":2990000`) {
		t.Fatal(string(b))
	}
	var p Price
	if err := json.Unmarshal([]byte(`{"amount":2990000,"currency":"VND","note":""}`), &p); err != nil || !p.Amount.Equal(decimal.NewFromInt(2990000)) {
		t.Fatal(err, p)
	}
}

func TestLicense(t *testing.T) {
	at := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	ok := Asset{AssetType: AssetMusic, License: json.RawMessage(`{"source":"Artlist","license_type":"commercial"}`)}
	expired := Asset{AssetType: AssetMusic, License: json.RawMessage(`{"source":"Artlist","license_type":"commercial","expires_at":"2026-10-18"}`)}
	incomplete := Asset{AssetType: AssetMusic, License: json.RawMessage(`{"source":"Artlist"}`)}
	none := Asset{AssetType: AssetMusic}
	if !ok.LicenseUsableAt(at) || expired.LicenseUsableAt(at) || incomplete.LicenseUsableAt(at) || none.LicenseUsableAt(at) {
		t.Fatal("license usability wrong")
	}
	var v kernel.ValidationErrors
	License{ExpiresAt: "x"}.Validate(&v, "license")
	if len(issues(v.Err())) != 3 {
		t.Fatal(issues(v.Err()))
	}
}
