package workflow

import (
	"fmt"
	"slices"
	"testing"
)

func render(refs []Ref) []string {
	var out []string
	for _, r := range refs {
		s := fmt.Sprintf("%s.%s@%d:%d", r.Context, r.Name, r.Line, r.Column)
		if r.Define {
			s = fmt.Sprintf("define %s/%s %s@%d:%d", r.Level, r.Job, r.Name, r.Line, r.Column)
		}
		out = append(out, s)
	}
	return out
}

func TestParse(t *testing.T) {
	src := `name: deploy
on: push
env:
  NODE_VERSION: "20"
  # ${{ secrets.IN_COMMENT }}
jobs:
  deploy:
    if: vars.DEPLOY_ENABLED == 'true' && secrets.DEPLOY_KEY != ''
    runs-on: ubuntu-latest
    env:
      REGION: ${{ vars.AWS_REGION }}
    steps:
      - uses: actions/checkout@v4
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
      - run: ./deploy.sh "${{ secrets.DEPLOY_KEY }}" ${{ secrets['API_TOKEN'] }}
        env:
          DATABASE_URL: ${{ secrets.DATABASE_URL }}
          OUT: ${{ steps.build.outputs.env.FAKE }}
          FROM_ENV: ${{ env.NODE_VERSION }}
`
	f := Parse([]byte(src), ".github/workflows/deploy.yml")
	if f.Problem != nil {
		t.Fatal(f.Problem)
	}
	want := []string{
		"define workflow/ NODE_VERSION@4:3",
		"define job/deploy REGION@11:7",
		"define step/deploy DATABASE_URL@18:11",
		"define step/deploy OUT@19:11",
		"define step/deploy FROM_ENV@20:11",
		"vars.DEPLOY_ENABLED@8:9",
		"secrets.DEPLOY_KEY@8:42",
		"vars.AWS_REGION@11:19",
		"secrets.DEPLOY_KEY@16:31",
		"secrets.API_TOKEN@16:58",
		"secrets.DATABASE_URL@18:29",
		"env.NODE_VERSION@20:25",
	}
	if got := render(f.Refs); !slices.Equal(got, want) {
		t.Errorf("got\n%v\nwant\n%v", got, want)
	}
}

func TestParseProblem(t *testing.T) {
	if f := Parse([]byte("jobs:\n  a: [\n"), "w.yml"); f.Problem == nil {
		t.Fatal("want problem")
	}
	if f := Parse(nil, "w.yml"); f.Problem != nil || len(f.Refs) != 0 {
		t.Errorf("empty file: %+v", f)
	}
}

// Found by fuzzing: a mapping (or alias) used as a key under env: produced a
// definition with an empty name.
func TestNonScalarEnvKeysAreIgnored(t *testing.T) {
	for _, src := range []string{"env:  {{}}\n", "env:\n  ? {a: b}\n  : c\n  \"\": d\n", "a: &k {x: y}\nenv:\n  *k : v\n"} {
		for _, r := range Parse([]byte(src), "w.yml").Refs {
			if r.Name == "" {
				t.Errorf("empty-name reference %+v from %q", r, src)
			}
		}
	}
}
