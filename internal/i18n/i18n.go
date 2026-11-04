// Package i18n holds the small RU/EN text selector used by the CLI.
package i18n

// Text returns Russian by default and English when lang is "en".
func Text(lang, ru, en string) string {
	if lang == "en" {
		return en
	}
	return ru
}
