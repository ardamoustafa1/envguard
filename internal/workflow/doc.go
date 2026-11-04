// Package workflow parses GitHub Actions workflows: env: keys at workflow,
// job and step level, and ${{ secrets.X }}, ${{ vars.X }} and ${{ env.X }}
// references (also in if: conditions), with positions. GITHUB_TOKEN is
// ignored.
package workflow
