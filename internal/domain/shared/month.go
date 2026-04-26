package shared

import "time"

type Month struct {
	year  int
	month time.Month
}

func NewMonth(year int, month time.Month) Month {
	return Month{year: year, month: month}
}

func (m Month) Year() int {
	return m.year
}

func (m Month) Month() time.Month {
	return m.month
}
