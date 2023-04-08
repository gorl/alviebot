package core

import (
	"fmt"
	"strconv"
	"strings"
)

type Renderer struct {
	currencyConverter CurrencyConverter
}

func NewRenderer(converter CurrencyConverter) *Renderer {
	return &Renderer{currencyConverter: converter}
}

func (r *Renderer) Render(template string) string {
	if !IsTemplate(template) {
		return template
	}
	matches := priceRe.FindAllString(template, -1)

	for _, match := range matches {
		formatted := r.formatPrice(match)
		template = strings.ReplaceAll(template, match, formatted)
	}
	return template
}

func (r *Renderer) formatPrice(priceToken string) string {
	priceString := strings.TrimLeft(priceToken, "$price:")
	priceUAH, err := strconv.ParseFloat(priceString, 64)
	if err != nil {
		return priceToken
	}

	priceRub := r.currencyConverter.Convert(priceUAH, uahCode, rurCode)

	return fmt.Sprintf("%.2f₴ (%.2f₽)", priceUAH, priceRub)
}
