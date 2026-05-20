package main

import (
	"log"

	"github.com/redstone-md/shield/app/observability"
)

func debugLogFields(label string, value any) {
	log.Printf("[DEBUG] %s:\n%s", label, observability.FormatFields(value))
}
