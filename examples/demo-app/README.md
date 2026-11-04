# demo-app

A small Node.js API with a Python worker, Docker Compose and a deploy
workflow. Its configuration is **broken on purpose** so you can see what
EnvGuard reports. It is also the source of every example in EnvGuard's README,
and a test keeps those examples in sync with real output.

```bash
cd examples/demo-app
envguard               # doctor: the full report
envguard check         # compact, exit code 1
envguard explain EG003
envguard diff
```

Deliberate problems include a variable missing in production, a variable the
code reads that nothing declares, committed env files, a credential in
`.env.example`, an undefined Compose variable, staging/production drift,
invalid and unsafe values, a duplicate key, a syntax error and an unresolved
`${...}` reference.

Every credential here is an obvious fake (`demo-only-pa55word`). The env files
are committed only because this is a demo; in a real repository they belong
in `.gitignore`.
