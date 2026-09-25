package geoctx

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type AccountContext struct{ Country, CountryCode, Timezone, Locale, Language string }

func (g AccountContext) Valid() bool { return g.Timezone != "" && g.Locale != "" }
func (g AccountContext) AcceptLanguage() string {
	if g.Locale == "" {
		return ""
	}
	lang := g.Language
	if lang == "" {
		lang = strings.Split(g.Locale, "-")[0]
	}
	return g.Locale + "," + lang + ";q=0.9"
}
func ForAccount(ctx context.Context, db *sql.DB, accountID string) (AccountContext, error) {
	var g AccountContext
	err := db.QueryRowContext(ctx, `SELECT COALESCE(country,''),COALESCE(preferred_country_code,''),COALESCE(timezone,''),COALESCE(locale,'') FROM account_network_identities WHERE account_id=$1`, accountID).Scan(&g.Country, &g.CountryCode, &g.Timezone, &g.Locale)
	if err != nil {
		return g, err
	}
	if g.Locale == "" {
		g.Locale = LocaleForCountry(g.CountryCode)
	}
	g.Language = strings.Split(g.Locale, "-")[0]
	if !g.Valid() {
		return g, errors.New("account geo context incomplete")
	}
	return g, nil
}
