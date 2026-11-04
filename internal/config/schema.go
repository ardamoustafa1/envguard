package config

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
)

// SchemaID is the published location of the JSON Schema.
const SchemaID = "https://raw.githubusercontent.com/ardamoustafa1/envguard/main/docs/envguard.schema.json"

// JSONSchema returns the JSON Schema (draft 2020-12) for .envguard.yaml,
// generated from the File struct and its tags. docs/envguard.schema.json is
// this output; TestJSONSchemaInSync keeps them identical.
func JSONSchema() ([]byte, error) {
	s := schemaFor(reflect.TypeFor[File](), reflect.StructTag(""))
	s["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	s["$id"] = SchemaID
	s["title"] = "EnvGuard configuration (.envguard.yaml)"
	s["description"] = "Optional configuration for envguard. Every key is optional except version."
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type jsonSchema = map[string]any

// schemaFor builds the schema of t. tag is the struct tag of the field that
// holds t; enum and pattern tags apply to the innermost string.
func schemaFor(t reflect.Type, tag reflect.StructTag) jsonSchema {
	s := jsonSchema{}
	if d := tag.Get("desc"); d != "" {
		s["description"] = d
	}
	switch t.Kind() {
	case reflect.Pointer:
		inner := schemaFor(t.Elem(), tag)
		for k, v := range inner {
			s[k] = v
		}
	case reflect.Struct:
		props := jsonSchema{}
		var required []string
		for i := range t.NumField() {
			f := t.Field(i)
			name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
			if name == "" || name == "-" {
				continue
			}
			props[name] = schemaFor(f.Type, f.Tag)
			if f.Tag.Get("required") == "true" {
				required = append(required, name)
			}
		}
		s["type"] = "object"
		s["properties"] = props
		s["additionalProperties"] = false
		if len(required) > 0 {
			s["required"] = required
		}
	case reflect.Map:
		s["type"] = "object"
		if p := tag.Get("keys"); p != "" {
			s["propertyNames"] = jsonSchema{"pattern": p}
		}
		s["additionalProperties"] = schemaFor(t.Elem(), leafTag(tag))
	case reflect.Slice:
		s["type"] = "array"
		s["items"] = schemaFor(t.Elem(), leafTag(tag))
	case reflect.String:
		s["type"] = "string"
		if e := tag.Get("enum"); e != "" {
			s["enum"] = enumValues(e)
		}
		if p := tag.Get("pattern"); p != "" {
			s["pattern"] = p
		}
	case reflect.Int:
		s["type"] = "integer"
		if c := tag.Get("const"); c != "" {
			n, err := strconv.Atoi(c)
			if err != nil {
				panic("config: bad const tag " + c) // programming error, caught by tests
			}
			s["const"] = n
		}
	case reflect.Float64:
		s["type"] = "number"
	case reflect.Bool:
		s["type"] = "boolean"
	default:
		panic("config: unsupported schema type " + t.String()) // programming error, caught by tests
	}
	return s
}

// leafTag keeps only the tags that describe the element of a slice or map.
func leafTag(tag reflect.StructTag) reflect.StructTag {
	var parts []string
	for _, k := range []string{"enum", "pattern"} {
		if v := tag.Get(k); v != "" {
			parts = append(parts, k+":"+strconv.Quote(v))
		}
	}
	return reflect.StructTag(strings.Join(parts, " "))
}
