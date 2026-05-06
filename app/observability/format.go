package observability

import (
	"fmt"
	"reflect"
	"strings"
)

const fieldIndent = "  "

// FormatFields renders exported struct fields one per line for readable debug logs.
func FormatFields(v any) string {
	value := reflect.ValueOf(v)
	value = indirectValue(value)
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return fmt.Sprintf("%+v", v)
	}
	var b strings.Builder
	writeStructFields(&b, value, "")
	return strings.TrimRight(b.String(), "\n")
}

func writeStructFields(b *strings.Builder, value reflect.Value, indent string) {
	valueType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		field := valueType.Field(i)
		if field.PkgPath != "" {
			continue
		}
		fieldValue := value.Field(i)
		writeField(b, field.Name, fieldValue, indent)
	}
}

func writeField(b *strings.Builder, name string, value reflect.Value, indent string) {
	value = indirectInterface(value)
	if value.IsValid() && value.Kind() == reflect.Struct {
		b.WriteString(indent)
		b.WriteString(name)
		b.WriteString(":\n")
		writeStructFields(b, value, indent+fieldIndent)
		return
	}
	b.WriteString(indent)
	b.WriteString(name)
	b.WriteString(": ")
	b.WriteString(formatFieldValue(value))
	b.WriteString("\n")
}

func formatFieldValue(value reflect.Value) string {
	if !value.IsValid() {
		return "<nil>"
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "<nil>"
		}
		return "*" + value.Type().Elem().String()
	}
	if !value.CanInterface() {
		return fmt.Sprintf("%v", value)
	}
	return fmt.Sprintf("%v", value.Interface())
}

func indirectValue(value reflect.Value) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return value
		}
		value = value.Elem()
	}
	return value
}

func indirectInterface(value reflect.Value) reflect.Value {
	for value.IsValid() && value.Kind() == reflect.Interface {
		if value.IsNil() {
			return value
		}
		value = value.Elem()
	}
	return value
}
