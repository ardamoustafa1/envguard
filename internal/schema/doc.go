// Package schema implements the value types and constraints of the
// `variables:` section of .envguard.yaml: string, int, float, bool, url,
// port, email, hostname, duration, enum, json and regex, with min/max,
// min_length/max_length, schemes, values and pattern. It does not depend on
// the config package (config uses it to validate defaults and production
// rules). Check reasons never contain the checked value.
package schema
