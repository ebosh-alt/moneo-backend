package http

import (
	"bytes"
	"encoding/json"
	"errors"

	"moneo/internal/domain/shared"
)

var ErrMoneyAmountMustBeString = errors.New("money amount must be a json string")

type DecimalString string

func (d *DecimalString) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) < 2 || trimmed[0] != '"' || trimmed[len(trimmed)-1] != '"' {
		return ErrMoneyAmountMustBeString
	}

	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return ErrMoneyAmountMustBeString
	}

	*d = DecimalString(value)
	return nil
}

func ParseMoneyFromREST(amount DecimalString, currencyCode string, requireNonNegative bool) (shared.Money, error) {
	currency, err := shared.ParseCurrency(currencyCode)
	if err != nil {
		return shared.Money{}, err
	}

	if requireNonNegative {
		return shared.ParseMoneyDecimalNonNegative(string(amount), currency)
	}

	return shared.ParseMoneyDecimal(string(amount), currency)
}

func ParseBalanceMoneyFromREST(amount DecimalString, currencyCode string) (shared.Money, error) {
	return ParseMoneyFromREST(amount, currencyCode, true)
}

func ParseInitialBalanceMoneyFromREST(amount DecimalString, currencyCode string) (shared.Money, error) {
	return ParseMoneyFromREST(amount, currencyCode, true)
}

func FormatMoneyToREST(money shared.Money) (string, error) {
	return shared.FormatMoneyDecimal(money)
}
