package main

import (
	"log"

	"github.com/umputun/tg-spam/app/observability"
)

func debugLogFields(label string, value any) {
	log.Printf("[DEBUG] %s:\n%s", label, observability.FormatFields(value))
}
