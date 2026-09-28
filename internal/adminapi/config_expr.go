package adminapi

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var errConfigExpr = errors.New("invalid numeric expression")

func parseProductExpr(s string) (int64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if s == "" {
		return 0, errConfigExpr
	}
	parts := strings.Split(s, "*")
	result := int64(1)
	for _, p := range parts {
		if p == "" {
			return 0, errConfigExpr
		}
		n, e := strconv.ParseInt(p, 10, 64)
		if e != nil || n < 0 {
			return 0, errConfigExpr
		}
		if n != 0 && result > math.MaxInt64/n {
			return 0, errConfigExpr
		}
		result *= n
	}
	return result, nil
}

func parseQuotaMB(s string) (int64, error) {
	n, e := parseProductExpr(s)
	if e != nil {
		return 0, e
	}
	if n > math.MaxInt64/1048576 {
		return 0, errConfigExpr
	}
	return n * 1048576, nil
}
func parseLifetimeMinutes(s string) (int, error) {
	n, e := parseProductExpr(s)
	if e != nil || n > math.MaxInt32/60 {
		return 0, errConfigExpr
	}
	return int(n * 60), nil
}
