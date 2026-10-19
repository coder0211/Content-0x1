package domain

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Product statuses.
const (
	ProductActive   = "active"
	ProductArchived = "archived"
)

// Offer validity relative to a date.
const (
	OfferActive   = "active"
	OfferExpired  = "expired"
	OfferUpcoming = "upcoming"
)

// DateLayout is the wire format for calendar dates.
const DateLayout = "2006-01-02"

// businessTZ decides which calendar day "today" is (Vietnam, UTC+7, no DST).
var businessTZ = time.FixedZone("ICT", 7*3600)

// Today returns the business calendar date of t as YYYY-MM-DD.
func Today(t time.Time) string { return t.In(businessTZ).Format(DateLayout) }

// ValidDate reports whether s is a YYYY-MM-DD date.
func ValidDate(s string) bool {
	_, err := time.Parse(DateLayout, s)
	return err == nil
}

// Price is a verified product price. Amount is written as a JSON number.
type Price struct {
	Amount   decimal.Decimal `json:"amount"`
	Currency string          `json:"currency"`
	Note     string          `json:"note"`
}

// MarshalJSON writes amount as a number (SPEC-04 example).
func (p Price) MarshalJSON() ([]byte, error) {
	type alias struct {
		Amount   json.RawMessage `json:"amount"`
		Currency string          `json:"currency"`
		Note     string          `json:"note"`
	}
	return json.Marshal(alias{Amount: json.RawMessage(p.Amount.String()), Currency: p.Currency, Note: p.Note})
}

// Offer is a time-boxed offer; dates are inclusive YYYY-MM-DD, empty = open.
type Offer struct {
	Text      string `json:"text"`
	ValidFrom string `json:"valid_from,omitempty"`
	ValidTo   string `json:"valid_to,omitempty"`
}

// StatusAt returns active | expired | upcoming for the business day of at.
func (o Offer) StatusAt(at time.Time) string {
	today := Today(at)
	switch {
	case o.ValidTo != "" && o.ValidTo < today:
		return OfferExpired
	case o.ValidFrom != "" && o.ValidFrom > today:
		return OfferUpcoming
	default:
		return OfferActive
	}
}

// Proof is social proof; only verified proof reaches prompts.
type Proof struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Source   string `json:"source"`
	Verified bool   `json:"verified"`
}

// Facts is products.facts (SPEC-04 §2.2).
type Facts struct {
	USP      []string `json:"usp"`
	Price    *Price   `json:"price"`
	Offers   []Offer  `json:"offers"`
	Policies []string `json:"policies"`
	Proof    []Proof  `json:"proof"`
}

// Normalize trims text and replaces nil slices with empty ones.
func (f Facts) Normalize() Facts {
	f.USP = cleanList(f.USP)
	f.Policies = cleanList(f.Policies)
	offers := make([]Offer, 0, len(f.Offers))
	for _, o := range f.Offers {
		offers = append(offers, Offer{Text: strings.TrimSpace(o.Text), ValidFrom: strings.TrimSpace(o.ValidFrom), ValidTo: strings.TrimSpace(o.ValidTo)})
	}
	f.Offers = offers
	proof := make([]Proof, 0, len(f.Proof))
	for _, p := range f.Proof {
		proof = append(proof, Proof{Type: strings.TrimSpace(p.Type), Text: strings.TrimSpace(p.Text), Source: strings.TrimSpace(p.Source), Verified: p.Verified})
	}
	f.Proof = proof
	if f.Price != nil {
		p := *f.Price
		p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))
		p.Note = strings.TrimSpace(p.Note)
		f.Price = &p
	}
	return f
}

// ProofTypes are the accepted proof kinds.
var ProofTypes = []string{"testimonial", "statistic", "award", "certification", "media", "other"}

// Validate reports every problem of normalized facts under prefix.
func (f Facts) Validate(v *kernel.ValidationErrors, prefix string) {
	checkList(v, prefix+".usp", f.USP)
	checkList(v, prefix+".policies", f.Policies)
	if f.Price != nil {
		if f.Price.Amount.IsNegative() {
			v.Add(prefix+".price.amount", "negative")
		}
		if len(f.Price.Currency) != 3 {
			v.Add(prefix+".price.currency", "invalid")
		}
	}
	if len(f.Offers) > MaxFactsEntries {
		v.Add(prefix+".offers", "too_many")
	}
	for i, o := range f.Offers {
		of := fmt.Sprintf("%s.offers[%d]", prefix, i)
		if o.Text == "" {
			v.Add(of+".text", "required")
		}
		checkText(v, of+".text", o.Text, MaxItemLen)
		if o.ValidFrom != "" && !ValidDate(o.ValidFrom) {
			v.Add(of+".valid_from", "invalid_date")
		}
		if o.ValidTo != "" && !ValidDate(o.ValidTo) {
			v.Add(of+".valid_to", "invalid_date")
		}
		if o.ValidFrom != "" && o.ValidTo != "" && o.ValidFrom > o.ValidTo {
			v.Add(of+".valid_to", "before_valid_from")
		}
	}
	if len(f.Proof) > MaxFactsEntries {
		v.Add(prefix+".proof", "too_many")
	}
	for i, p := range f.Proof {
		pf := fmt.Sprintf("%s.proof[%d]", prefix, i)
		if p.Text == "" {
			v.Add(pf+".text", "required")
		}
		checkText(v, pf+".text", p.Text, MaxTextLen)
		if p.Type != "" && !slices.Contains(ProofTypes, p.Type) {
			v.Add(pf+".type", "invalid")
		}
	}
}

// ParseFacts decodes stored facts tolerantly.
func ParseFacts(raw json.RawMessage) Facts {
	var f Facts
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &f)
	}
	return f.Normalize()
}

// PromptFacts keeps only what generation may use at time at: offers valid
// that day and verified proof (FR-CNT-06, ACS-022).
func (f Facts) PromptFacts(at time.Time) Facts {
	out := f
	out.Offers = make([]Offer, 0, len(f.Offers))
	for _, o := range f.Offers {
		if o.StatusAt(at) == OfferActive {
			out.Offers = append(out.Offers, o)
		}
	}
	out.Proof = make([]Proof, 0, len(f.Proof))
	for _, p := range f.Proof {
		if p.Verified {
			out.Proof = append(out.Proof, p)
		}
	}
	return out
}

// Product is a products row.
type Product struct {
	ID          kernel.ID
	BrandID     kernel.ID
	Name        string
	Description string
	Facts       Facts
	Status      string
	CreatedBy   *kernel.ID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
