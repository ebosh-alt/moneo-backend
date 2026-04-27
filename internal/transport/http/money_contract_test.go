package http

import (
	"encoding/json"
	"testing"

	"moneo/internal/domain/shared"
)

func TestDecimalStringRejectsNonStringJSONValues(t *testing.T) {
	testCases := []string{
		`{"amount":100.50}`,
		`{"amount":100}`,
		`{"amount":null}`,
		`{"amount":true}`,
	}

	for _, payload := range testCases {
		t.Run(payload, func(t *testing.T) {
			var request struct {
				Amount DecimalString `json:"amount"`
			}
			err := json.Unmarshal([]byte(payload), &request)
			if err == nil {
				t.Fatalf("expected unmarshal error for payload %s", payload)
			}
		})
	}
}

func TestParseMoneyFromRESTForBalanceRequiresNonNegativeAmount(t *testing.T) {
	_, err := ParseBalanceMoneyFromREST(DecimalString("-100.00"), "RUB")
	if err == nil {
		t.Fatal("expected error for negative balance amount")
	}
}

func TestParseMoneyFromRESTParsesValidMoneyContract(t *testing.T) {
	money, err := ParseMoneyFromREST(DecimalString("120000.99"), "RUB", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if money.MinorUnits() != 12_000_099 {
		t.Fatalf("expected 12000099 minor units, got %d", money.MinorUnits())
	}
	if money.Currency() != shared.CurrencyRUB {
		t.Fatalf("expected RUB, got %s", money.Currency())
	}
}

func TestFormatMoneyToRESTReturnsCanonicalRUBAmount(t *testing.T) {
	amount, err := FormatMoneyToREST(shared.NewMoney(10_000, shared.CurrencyRUB))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amount != "100.00" {
		t.Fatalf("expected 100.00, got %q", amount)
	}
}
