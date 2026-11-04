package rules

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/parser"
	"github.com/ardamoustafa1/envguard/internal/project"
	"github.com/ardamoustafa1/envguard/internal/schema"
)

func init() {
	Register(unsafeProductionValue{base{"EG015", "unsafe-production-value", findings.Warning}})
}

// unsafeProductionValue checks values in the production environment only
// (environment "production"; prod is normalized to it). It is never active
// by default; it reports:
//
//   - per-variable `production:` rules from the schema: must_be,
//     must_not_be (compared by type, so off == false) and pattern;
//   - with `profiles: [production-safety]`, framework-aware built-ins:
//     DEBUG / DJANGO_DEBUG / APP_DEBUG (Laravel) / FLASK_DEBUG truthy,
//     NODE_ENV or RAILS_ENV not "production", APP_ENV local/development/dev,
//     and *_URL / *_HOST values pointing to localhost, 127.0.0.1, 0.0.0.0
//     or ::1 (in --process-env mode only for variables declared in a
//     template or the schema).
//
// Empty values are ignored (EG005's business).
type unsafeProductionValue struct{ base }

func (unsafeProductionValue) Check(p *project.Project) []findings.Finding {
	profile := p.Settings != nil && slices.Contains(p.Settings.Profiles, config.ProfileProductionSafety)
	var out []findings.Finding
	for _, f := range p.EnvFiles {
		if f.IsTemplate() || f.Environment != discovery.EnvProduction || !p.EnvSelected(f.Environment) {
			continue
		}
		for _, e := range lastEntries(f) {
			if strings.TrimSpace(e.Value) == "" {
				continue
			}
			if sv := p.SchemaVar(e.Key); sv != nil && sv.Config.Production != nil {
				// An explicit schema rule replaces the profile's built-in check.
				out = append(out, schemaProductionRule(p, f, e, sv)...)
				continue
			}
			if profile {
				if fi, ok := profileCheck(p, f, e); ok {
					out = append(out, fi)
				}
			}
		}
	}
	return out
}

func schemaProductionRule(p *project.Project, f *project.EnvFile, e parser.Entry, sv *project.SchemaVar) []findings.Finding {
	r := sv.Config.Production
	loc := p.Schema.Location(e.Key)
	ref := fmt.Sprintf("%s:%d", loc.File, loc.Line)
	var out []findings.Finding
	add := func(problem, fix string) {
		out = append(out, prodFinding(f, e, problem,
			fmt.Sprintf("The schema (%s) constrains %s in production; violating it usually means a development "+
				"or test setting is about to be deployed.", ref, e.Key),
			fix, &findings.Location{File: loc.File, Line: loc.Line, Column: loc.Column}))
	}
	shown := ""
	if !p.Sensitive(e.Key) {
		shown = " but is " + p.DisplayValue(e.Key, e.Value)
	}
	if r.MustBe != "" && !sv.Spec.Equal(e.Value, r.MustBe) {
		add(fmt.Sprintf("%s must be %s in production%s", e.Key, r.MustBe, shown),
			fmt.Sprintf("Set `%s=%s` in %s.", e.Key, r.MustBe, f.Path))
	}
	if r.MustNotBe != "" && sv.Spec.Equal(e.Value, r.MustNotBe) {
		add(fmt.Sprintf("%s must not be %s in production", e.Key, r.MustNotBe),
			fmt.Sprintf("Change %s in %s to a production-appropriate value.", e.Key, f.Path))
	}
	if r.Pattern != "" {
		if re, err := regexp.Compile(r.Pattern); err == nil && !re.MatchString(e.Value) {
			add(fmt.Sprintf("%s does not match the production pattern %q", e.Key, r.Pattern),
				fmt.Sprintf("Set %s in %s to a value matching %q.", e.Key, f.Path, r.Pattern))
		}
	}
	return out
}

var localHosts = []string{"localhost", "127.0.0.1", "0.0.0.0", "::1"}

// profileCheck implements the production-safety profile for one entry.
func profileCheck(p *project.Project, f *project.EnvFile, e parser.Entry) (findings.Finding, bool) {
	key, v := e.Key, strings.TrimSpace(e.Value)
	truthy := func() bool { b, ok := schema.ParseBool(v); return ok && b }
	switch {
	case (key == "DEBUG" || key == "DJANGO_DEBUG" || key == "APP_DEBUG" || key == "FLASK_DEBUG") && truthy():
		framework := map[string]string{"DEBUG": "", "DJANGO_DEBUG": "Django ", "APP_DEBUG": "Laravel ", "FLASK_DEBUG": "Flask "}[key]
		return prodFinding(f, e,
			fmt.Sprintf("%s is enabled in production", key),
			fmt.Sprintf("%sdebug mode in production exposes stack traces, settings and sometimes an interactive "+
				"console to anyone who triggers an error.", framework),
			fmt.Sprintf("Set `%s=false` in %s.", key, f.Path), nil), true
	case (key == "NODE_ENV" || key == "RAILS_ENV") && v != "production":
		return prodFinding(f, e,
			fmt.Sprintf("%s is %s, but production must use \"production\"", key, p.DisplayValue(key, v)),
			"Frameworks switch off caching, minification and security defaults outside \"production\", and some "+
				"enable verbose errors.",
			fmt.Sprintf("Set `%s=production` in %s.", key, f.Path), nil), true
	case key == "APP_ENV" && slices.Contains([]string{"local", "development", "dev"}, strings.ToLower(v)):
		return prodFinding(f, e,
			fmt.Sprintf("APP_ENV is %s in production", p.DisplayValue(key, v)),
			"Laravel and similar frameworks enable development behavior (debug pages, relaxed security) for "+
				"local and development environments.",
			fmt.Sprintf("Set `APP_ENV=production` in %s.", f.Path), nil), true
	case (strings.HasSuffix(key, "_URL") || strings.HasSuffix(key, "_HOST")) && pointsToLocalhost(key, v) &&
		(!p.ProcessEnv || declaredAnywhere(p, key)):
		return prodFinding(f, e,
			fmt.Sprintf("%s points to a local address in production", key),
			"Inside a production container or server, localhost is the machine itself, not your database or API; "+
				"this is almost always a development value that slipped through.",
			fmt.Sprintf("Set %s in %s to the production host.", key, f.Path), nil), true
	}
	return findings.Finding{}, false
}

// declaredAnywhere reports whether a template or the schema declares key.
// In --process-env mode the process holds many variables the app does not
// own (set by the shell, CI runner or platform); the *_URL / *_HOST check
// only looks at declared ones.
func declaredAnywhere(p *project.Project, key string) bool {
	if p.SchemaVar(key) != nil {
		return true
	}
	for _, f := range p.EnvFiles {
		if f.IsTemplate() {
			if _, ok := f.Lookup(key); ok {
				return true
			}
		}
	}
	return false
}

func pointsToLocalhost(key, v string) bool {
	host := v
	if strings.HasSuffix(key, "_URL") {
		u, err := url.Parse(v)
		if err != nil || u.Host == "" {
			return false
		}
		host = u.Hostname()
	} else if h, _, ok := strings.Cut(v, ":"); ok && !strings.Contains(v, "::") {
		host = h
	}
	return slices.Contains(localHosts, strings.ToLower(strings.Trim(host, "[]")))
}

func prodFinding(f *project.EnvFile, e parser.Entry, msg, why, fix string, related *findings.Location) findings.Finding {
	fi := findings.Finding{
		Message:  msg + " (" + strings.TrimPrefix(where(f), "the ") + ")",
		Variable: e.Key,
		File:     f.Path,
		Line:     e.ValuePos.Line,
		Column:   e.ValuePos.Column,
		Why:      why,
		Fix:      fix,
	}
	if related != nil {
		fi.Related = []findings.Location{*related}
	}
	return fi
}
