package geoctx

func (g AccountContext) Env() []string {
	return []string{"TZ=" + g.Timezone, "LANG=" + g.Locale + ".UTF-8", "LC_ALL=" + g.Locale + ".UTF-8", "ACCOUNT_LOCALE=" + g.Locale, "ACCOUNT_LANGUAGE=" + g.Language, "ACCOUNT_ACCEPT_LANGUAGE=" + g.AcceptLanguage()}
}
