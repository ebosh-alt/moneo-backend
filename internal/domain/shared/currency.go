package shared

import "errors"

type Currency string

const (
	CurrencyRUB Currency = "RUB"
	CurrencyUSD Currency = "USD"
	CurrencyEUR Currency = "EUR"
)

var ErrUnsupportedCurrency = errors.New("unsupported currency")

func (c Currency) String() string {
	return string(c)
}

func ParseCurrency(value string) (Currency, error) {
	currency := Currency(value)
	if !currency.IsSupported() {
		return "", ErrUnsupportedCurrency
	}

	return currency, nil
}

func (c Currency) IsSupported() bool {
	switch c {
	case CurrencyRUB, CurrencyUSD, CurrencyEUR:
		return true
	default:
		return false
	}
}
