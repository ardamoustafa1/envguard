package scanner

import (
	"bytes"
	"reflect"
	"regexp"
	"slices"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/discovery"
)

var varNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// FuzzScanFile runs every language's detectors over arbitrary bytes. The
// first byte picks the language. It must never panic or hang, be
// deterministic, return sorted usages with sane positions, and never report
// a name that is not a plausible variable name (precision over recall: a
// phantom usage is a bug).
func FuzzScanFile(f *testing.F) {
	langs := discovery.Languages()
	seeds := map[discovery.Language][]string{
		discovery.LangJavaScript: {
			"const {A, B = 'd', ...rest} = process.env; x = process.env.C || 'x'; y = process.env[k]; `${process.env.D}`; /re/.test(1)",
			"// process.env.X\n/* process.env.Y */ \"process.env.Z\"; import.meta.env.VITE_A",
			"`${`${process.env.A}`}` + /[/]/ + `",
		},
		discovery.LangPython: {
			"import os\nos.environ['A']; os.getenv('B', 'd'); os.environ.get('C')\n'''os.environ['D']'''\nf\"{os.environ['E']}\"",
			"from os import environ\nenviron['A']\n\"\"\"",
		},
		discovery.LangGo: {
			"package p\nimport \"os\"\nvar x = os.Getenv(\"A\"); _, ok := os.LookupEnv(\"B\")\ntype C struct { D string `env:\"E\" envDefault:\"f\"` }\n`raw",
		},
		discovery.LangRuby:       {"ENV['A']; ENV.fetch('B', 'd'); ENV.fetch('C') { 1 }\n=begin\nENV['X']\n=end\n\"#{ENV['Y']}\""},
		discovery.LangPHP:        {"<?php getenv('A'); $_ENV['B']; $_SERVER['HTTP_X']; env('C', 'd'); ?><?= $_ENV['D'] ?>"},
		discovery.LangRust:       {"std::env::var(\"A\"); env!(\"B\"); option_env!(\"C\"); r#\"env!(\"X\")\"#; 'a'; '\\''; /* /* nested */ */"},
		discovery.LangJava:       {"System.getenv(\"A\"); System.getenv().getOrDefault(\"B\", \"d\"); /* System.getenv(\"X\") */"},
		discovery.LangKotlin:     {"System.getenv(\"A\") ?: \"d\"; \"${System.getenv(\"B\")}\""},
		discovery.LangCSharp:     {"Environment.GetEnvironmentVariable(\"A\"); @\"Environment.GetEnvironmentVariable(\"\"X\"\")\""},
		discovery.LangElixir:     {"System.get_env(\"A\", \"d\"); System.fetch_env!(\"B\"); Application.get_env(:a, :b)"},
		discovery.LangShell:      {"#!/bin/sh\necho ${A:-d} $B ${!C} \"${D:?}\"; X=${X:-1}; read Y; for Z in a; do :; done; echo '$Q'"},
		discovery.LangVue:        {"<template>{{ process.env.X }}</template><script>process.env.A</script>"},
		discovery.LangSvelte:     {"<script>process.env.A</script><p>{process.env.B}</p>"},
		discovery.LangTypeScript: {"const a: string = process.env.A!; export default defineConfig({ define: { b: import.meta.env.VITE_B } })"},
	}
	for i, l := range langs {
		for _, s := range seeds[l] {
			f.Add(byte(i), []byte(s))
		}
		f.Add(byte(i), []byte(""))
		f.Add(byte(i), []byte("\"\xff'`/*\\${"))
	}
	f.Fuzz(func(t *testing.T, sel byte, data []byte) {
		lang := langs[int(sel)%len(langs)]
		u1 := scanFile(lang, "fuzz/src", data)
		if !reflect.DeepEqual(u1, scanFile(lang, "fuzz/src", data)) {
			t.Fatal("scanning is not deterministic")
		}
		lines := bytes.Count(data, []byte("\n")) + 1
		if !slices.IsSortedFunc(u1, func(a, b Usage) int {
			switch {
			case a.Line != b.Line:
				return a.Line - b.Line
			case a.Column != b.Column:
				return a.Column - b.Column
			}
			return 0
		}) {
			t.Fatal("usages are not sorted")
		}
		for _, u := range u1 {
			if u.Line < 1 || u.Line > lines || u.Column < 1 {
				t.Fatalf("%s: position %d:%d outside a %d-line file", lang, u.Line, u.Column, lines)
			}
			if u.Dynamic {
				if u.Variable != "" {
					t.Fatal("dynamic usage with a name")
				}
				continue
			}
			if !varNameRE.MatchString(u.Variable) {
				t.Fatalf("%s: malformed variable name %q reported", lang, u.Variable)
			}
		}
	})
}
