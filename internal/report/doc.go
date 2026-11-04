// Package report renders analysis results in the pretty, doctor, json, sarif
// and github formats, plus the environment matrix (diff) and rule
// documentation (explain).
//
// Reporters only ever receive masked data: findings carry no values by
// contract (Message, Why and Fix never contain them; Evidence comes from
// internal/mask), and the matrix records presence, never values. Every
// renderer is a pure function of its input and Options, so the same input
// gives byte-identical output. The only non-deterministic datum, the run
// duration, is printed only when Options.ShowDuration is set (interactive
// terminals).
package report
