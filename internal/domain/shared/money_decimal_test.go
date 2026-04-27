package shared

import "testing"

func TestParseMinorUnitsDecimalNonNegativeAcceptsValidExamples(t *testing.T) {
	testCases := []struct {
		name     string
		amount   string
		expected int64
	}{
		{
			name:     "zero",
			amount:   "0",
			expected: 0,
		},
		{
			name:     "zero with fractional",
			amount:   "0.00",
			expected: 0,
		},
		{
			name:     "whole amount",
			amount:   "100",
			expected: 10_000,
		},
		{
			name:     "fractional amount",
			amount:   "100.50",
			expected: 10_050,
		},
		{
			name:     "large amount",
			amount:   "120000.99",
			expected: 12_000_099,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseMinorUnitsDecimalNonNegative(tc.amount, CurrencyRUB)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Fatalf("expected %d minor units, got %d", tc.expected, got)
			}
		})
	}
}

func TestParseMinorUnitsDecimalNonNegativeRejectsInvalidExamples(t *testing.T) {
	invalidAmounts := []string{
		"100,50",
		"100.555",
		"-100.00",
		"10 000.00",
		"₽100.00",
	}

	for _, amount := range invalidAmounts {
		t.Run(toTestName(amount), func(t *testing.T) {
			_, err := ParseMinorUnitsDecimalNonNegative(amount, CurrencyRUB)
			if err == nil {
				t.Fatalf("expected parse error for %q", amount)
			}
		})
	}
}

func TestFormatMinorUnitsDecimalReturnsCanonicalTwoDigitsForRUB(t *testing.T) {
	testCases := []struct {
		minorUnits int64
		expected   string
	}{
		{minorUnits: 0, expected: "0.00"},
		{minorUnits: 1, expected: "0.01"},
		{minorUnits: 10_000, expected: "100.00"},
		{minorUnits: 10_050, expected: "100.50"},
		{minorUnits: 12_000_099, expected: "120000.99"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			got, err := FormatMinorUnitsDecimal(tc.minorUnits, CurrencyRUB)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Fatalf("expected formatted amount %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestParseCurrencyAcceptsMVPContractCurrencies(t *testing.T) {
	testCases := []string{"RUB", "USD", "EUR"}
	for _, code := range testCases {
		t.Run(code, func(t *testing.T) {
			got, err := ParseCurrency(code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != code {
				t.Fatalf("expected %q, got %q", code, got.String())
			}
		})
	}
}

func toTestName(value any) string {
	if str, ok := value.(string); ok {
		if str == "" {
			return "empty"
		}
		return str
	}

	return "value"
}
