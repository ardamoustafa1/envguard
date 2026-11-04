package compose

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func interps(f *File) []string {
	var out []string
	for _, it := range f.Interpolations {
		s := fmt.Sprintf("%s@%d:%d", it.Variable, it.Line, it.Column)
		if it.HasDefault {
			s += "+d"
		}
		if it.Required {
			s += "!"
		}
		out = append(out, s)
	}
	return out
}

func TestInterpolations(t *testing.T) {
	src := `services:
  web:
    image: "app:${TAG:-latest}"
    command: echo $$NOT_A_VAR $${ALSO_NOT} ${REAL}
    ports:
      - "${PORT-8080}:80"
    environment:
      DB: postgres://${DB_USER}:${DB_PASS:?set it}@db/${DB_NAME?}
      ALT: ${OPT:+on}
    labels:
      bare: $BARE_VAR
      multi: >
        line one ${IN_BLOCK}
# ${IN_COMMENT} is not interpolated
`
	f := Parse([]byte(src), "docker-compose.yml")
	if f.Problem != nil {
		t.Fatal(f.Problem)
	}
	want := []string{
		"TAG@3:19+d", "REAL@4:46", "PORT@6:12+d",
		"DB_USER@8:24", "DB_PASS@8:35!", "DB_NAME@8:57!", "OPT@9:14+d",
		"BARE_VAR@11:14", "IN_BLOCK@13:20",
	}
	if got := interps(f); !slices.Equal(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

func TestEnvironmentForms(t *testing.T) {
	src := `x-common: &common
  SHARED: "1"
services:
  api:
    environment:
      <<: *common
      DATABASE_URL: postgres://db/app
      PASS_THROUGH:
  worker:
    environment:
      - QUEUE=jobs
      - FROM_SHELL
    env_file:
      - .env
      - path: ./config/optional.env
        required: false
      - path: ../outside.env
  single:
    env_file: app.env
`
	f := Parse([]byte(src), "deploy/compose.yaml")
	if f.Problem != nil {
		t.Fatal(f.Problem)
	}
	names := map[string][]string{}
	for _, s := range f.Services {
		for _, e := range s.Environment {
			n := e.Name
			if e.PassThrough {
				n += "(shell)"
			}
			names[s.Name] = append(names[s.Name], n)
		}
	}
	if got := names["api"]; !slices.Equal(got, []string{"DATABASE_URL", "PASS_THROUGH(shell)", "SHARED"}) {
		t.Errorf("api env = %v", got)
	}
	if got := names["worker"]; !slices.Equal(got, []string{"QUEUE", "FROM_SHELL(shell)"}) {
		t.Errorf("worker env = %v", got)
	}
	var files []string
	for _, s := range f.Services {
		for _, e := range s.EnvFiles {
			files = append(files, fmt.Sprintf("%s:%s:%v", s.Name, e.Path, e.Required))
		}
	}
	want := []string{"single:deploy/app.env:true", "worker:deploy/.env:true", "worker:deploy/config/optional.env:false", "worker:outside.env:true"}
	if !slices.Equal(files, want) {
		t.Errorf("env files = %v\nwant %v", files, want)
	}
	if f.AdjacentEnv() != "deploy/.env" || f.Dir != "deploy" {
		t.Errorf("AdjacentEnv = %s", f.AdjacentEnv())
	}
}

func TestParseProblemsAndEdgeCases(t *testing.T) {
	for _, src := range []string{"", "just a string", "services: 5", "services:\n  web:\n    environment: 5\n    env_file: {}\n"} {
		if f := Parse([]byte(src), "c.yml"); f.Problem != nil {
			t.Errorf("%q: unexpected problem %v", src, f.Problem)
		}
	}
	f := Parse([]byte("services:\n  web: [\n"), "c.yml")
	if f.Problem == nil || f.Problem.Line == 0 {
		t.Errorf("problem = %+v", f.Problem)
	}
}

// TestAliasBombIsLinear: walking never follows aliases, so a billion-laughs
// document is processed in linear time.
func TestAliasBombIsLinear(t *testing.T) {
	var b strings.Builder
	b.WriteString("a: &a [\"${X}\", \"${X}\"]\n")
	prev := "a"
	for i := range 30 {
		name := fmt.Sprintf("l%d", i)
		fmt.Fprintf(&b, "%s: &%s [*%s, *%s]\n", name, name, prev, prev)
		prev = name
	}
	f := Parse([]byte(b.String()), "bomb.yml")
	if len(f.Interpolations) != 2 {
		t.Errorf("interpolations = %d, want 2 (aliases must not be expanded)", len(f.Interpolations))
	}
}

func TestResolve(t *testing.T) {
	tests := map[string]string{"app.env": "d/app.env", "./x/.env": "d/x/.env", "../y.env": "y.env", "../../z.env": "",
		"/abs/.env": "", "${DIR}/.env": "", `C:\x.env`: ""}
	for in, want := range tests {
		if got := resolve("d", in); got != want {
			t.Errorf("resolve(d, %q) = %q, want %q", in, got, want)
		}
	}
}
