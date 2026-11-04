package main

import (
	"fmt"
	"os"
)

// os.Getenv("FAKE_IN_COMMENT") is documented here.
type Config struct {
	Port  int    `env:"REAL_PORT" envDefault:"8080"`
	DSN   string `env:"REAL_DSN,required"`
	Label string `json:"label"`
}

func load(key string) string {
	token := os.Getenv("REAL_TOKEN")
	if v, ok := os.LookupEnv("REAL_OPTIONAL"); ok {
		token += v
	}
	fmt.Println("os.Getenv(\"FAKE_IN_STRING\")")
	raw := `os.Getenv("FAKE_IN_RAW_STRING")`
	return token + raw + os.Getenv(key)
}
