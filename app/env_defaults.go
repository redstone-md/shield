package main

import (
	"os"
	"reflect"
)

func unsetEmptyOptionEnv(data any) func() {
	keys := make(map[string]struct{})
	collectOptionEnvKeys(reflect.TypeOf(data), "", keys)

	restore := make(map[string]string)
	for key := range keys {
		if val, ok := os.LookupEnv(key); ok && val == "" {
			restore[key] = val
			_ = os.Unsetenv(key)
		}
	}

	return func() {
		for key, val := range restore {
			_ = os.Setenv(key, val)
		}
	}
}

func collectOptionEnvKeys(t reflect.Type, prefix string, keys map[string]struct{}) {
	if t == nil {
		return
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if envName := field.Tag.Get("env"); envName != "" {
			keys[joinEnvKey(prefix, envName)] = struct{}{}
		}

		envNamespace := field.Tag.Get("env-namespace")
		if envNamespace == "" {
			continue
		}
		collectOptionEnvKeys(field.Type, joinEnvKey(prefix, envNamespace), keys)
	}
}

func joinEnvKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "_" + key
}
