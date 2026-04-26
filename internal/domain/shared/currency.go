package shared

type Currency string

const (
	CurrencyRUB Currency = "RUB"
	CurrencyUSD Currency = "USD"
	CurrencyEUR Currency = "EUR"
)

func (c Currency) String() string {
	return string(c)
}
