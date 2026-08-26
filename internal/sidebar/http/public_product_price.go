package http

import "strconv"

func formatMinorPrice(value int64) string {
	return strconv.FormatInt(value/100, 10) + "." + twoDigits(value%100)
}

func twoDigits(value int64) string {
	if value < 10 {
		return "0" + strconv.FormatInt(value, 10)
	}
	return strconv.FormatInt(value, 10)
}
