package shared

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var (
	ErrInvalidMoneyAmount      = errors.New("invalid money amount")
	ErrMoneyAmountOverflow     = errors.New("money amount overflow")
	ErrNegativeMoneyAmount     = errors.New("money amount must be non-negative")
	ErrTooManyFractionalDigits = errors.New("too many fractional digits")
)

func ParseMoneyDecimal(amount string, currency Currency) (Money, error) {
	minorUnits, err := ParseMinorUnitsDecimal(amount, currency)
	if err != nil {
		return Money{}, err
	}

	return NewMoney(minorUnits, currency), nil
}

func ParseMoneyDecimalNonNegative(amount string, currency Currency) (Money, error) {
	minorUnits, err := ParseMinorUnitsDecimalNonNegative(amount, currency)
	if err != nil {
		return Money{}, err
	}

	return NewMoney(minorUnits, currency), nil
}

func ParseMinorUnitsDecimal(amount string, currency Currency) (int64, error) {
	fractionalDigits, err := currencyFractionalDigits(currency)
	if err != nil {
		return 0, err
	}

	normalized := amount
	if normalized == "" || strings.TrimSpace(normalized) != normalized || strings.Contains(normalized, ",") {
		return 0, ErrInvalidMoneyAmount
	}

	isNegative := false
	if strings.HasPrefix(normalized, "-") {
		isNegative = true
		normalized = strings.TrimPrefix(normalized, "-")
	}

	parts := strings.Split(normalized, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, ErrInvalidMoneyAmount
	}
	if !isDigits(parts[0]) {
		return 0, ErrInvalidMoneyAmount
	}

	major, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, ErrMoneyAmountOverflow
		}
		return 0, ErrInvalidMoneyAmount
	}

	multiplier := int64Pow10(fractionalDigits)
	if major > math.MaxInt64/multiplier {
		return 0, ErrMoneyAmountOverflow
	}
	minor := major * multiplier

	if len(parts) == 2 {
		fracPart := parts[1]
		if fracPart == "" || !isDigits(fracPart) {
			return 0, ErrInvalidMoneyAmount
		}
		if len(fracPart) > fractionalDigits {
			return 0, ErrTooManyFractionalDigits
		}

		padded := fracPart + strings.Repeat("0", fractionalDigits-len(fracPart))
		fraction, fracErr := strconv.ParseInt(padded, 10, 64)
		if fracErr != nil {
			if errors.Is(fracErr, strconv.ErrRange) {
				return 0, ErrMoneyAmountOverflow
			}
			return 0, ErrInvalidMoneyAmount
		}
		if minor > math.MaxInt64-fraction {
			return 0, ErrMoneyAmountOverflow
		}
		minor += fraction
	}

	if isNegative {
		minor = -minor
	}

	return minor, nil
}

func ParseMinorUnitsDecimalNonNegative(amount string, currency Currency) (int64, error) {
	minorUnits, err := ParseMinorUnitsDecimal(amount, currency)
	if err != nil {
		return 0, err
	}
	if minorUnits < 0 {
		return 0, ErrNegativeMoneyAmount
	}

	return minorUnits, nil
}

func FormatMoneyDecimal(money Money) (string, error) {
	return FormatMinorUnitsDecimal(money.MinorUnits(), money.Currency())
}

func FormatMinorUnitsDecimal(minorUnits int64, currency Currency) (string, error) {
	fractionalDigits, err := currencyFractionalDigits(currency)
	if err != nil {
		return "", err
	}

	sign := ""
	digits := strconv.FormatInt(minorUnits, 10)
	if strings.HasPrefix(digits, "-") {
		sign = "-"
		digits = strings.TrimPrefix(digits, "-")
	}

	if len(digits) <= fractionalDigits {
		digits = strings.Repeat("0", fractionalDigits+1-len(digits)) + digits
	}

	major := digits[:len(digits)-fractionalDigits]
	fraction := digits[len(digits)-fractionalDigits:]

	return sign + major + "." + fraction, nil
}

func currencyFractionalDigits(currency Currency) (int, error) {
	switch currency {
	case CurrencyRUB, CurrencyUSD, CurrencyEUR:
		return 2, nil
	default:
		return 0, ErrUnsupportedCurrency
	}
}

func int64Pow10(power int) int64 {
	result := int64(1)
	for i := 0; i < power; i++ {
		result *= 10
	}
	return result
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}

	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}
