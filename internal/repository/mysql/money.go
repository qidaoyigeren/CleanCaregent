package mysql

import (
	"fmt"
	"strconv"
	"strings"
)

func parseDecimalCents(value string) (int64, error) {
	return parseScaledDecimal(value, 2)
}

func parseDecimalBasisPoints(value string) (int64, error) {
	return parseScaledDecimal(value, 4)
}

func parseScaledDecimal(value string, scale int) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(value, "-")
	parts := strings.SplitN(value, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse decimal whole part %q: %w", value, err)
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > scale {
		for _, digit := range fraction[scale:] {
			if digit != '0' {
				return 0, fmt.Errorf("decimal %q exceeds supported scale %d", value, scale)
			}
		}
		fraction = fraction[:scale]
	}
	fraction += strings.Repeat("0", scale-len(fraction))
	fractionValue := int64(0)
	if fraction != "" {
		fractionValue, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse decimal fraction %q: %w", value, err)
		}
	}
	multiplier := int64(1)
	for range scale {
		multiplier *= 10
	}
	result := whole*multiplier + fractionValue
	if negative {
		result = -result
	}
	return result, nil
}
