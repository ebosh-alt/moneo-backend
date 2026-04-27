package shared

import "errors"

var ErrCurrencyMismatch = errors.New("currency mismatch")

type Money struct {
	minorUnits int64
	currency   Currency
}

func NewMoney(minorUnits int64, currency Currency) Money {
	return Money{
		minorUnits: minorUnits,
		currency:   currency,
	}
}

func (m Money) MinorUnits() int64 {
	return m.minorUnits
}

func (m Money) Currency() Currency {
	return m.currency
}

func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}

	return Money{
		minorUnits: m.minorUnits + other.minorUnits,
		currency:   m.currency,
	}, nil
}

func (m Money) Negate() Money {
	return Money{
		minorUnits: -m.minorUnits,
		currency:   m.currency,
	}
}
