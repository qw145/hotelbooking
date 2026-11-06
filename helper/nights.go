package helper

import "time"

// NightsBetween counts the nights in a half-open [checkIn, checkOut) range.
func NightsBetween(checkIn, checkOut time.Time) int {
	in := truncateToDate(checkIn)
	out := truncateToDate(checkOut)
	days := int(out.Sub(in).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

func truncateToDate(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
