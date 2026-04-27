package shared

import "testing"

func TestMoneyRequiresMatchingCurrencyForAddition(t *testing.T) {
	rubles := NewMoney(100_00, CurrencyRUB)
	dollars := NewMoney(10_00, CurrencyUSD)

	_, err := rubles.Add(dollars)

	if err == nil {
		t.Fatal("expected currency mismatch error")
	}
}

func TestMoneyAddsMinorUnitsWhenCurrencyMatches(t *testing.T) {
	left := NewMoney(100_00, CurrencyRUB)
	right := NewMoney(25_50, CurrencyRUB)

	got, err := left.Add(right)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.MinorUnits() != 125_50 {
		t.Fatalf("expected 12550 minor units, got %d", got.MinorUnits())
	}
	if got.Currency() != CurrencyRUB {
		t.Fatalf("expected RUB, got %s", got.Currency())
	}
}
