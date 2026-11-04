# EnvGuard

> Your code knows which env vars it needs. EnvGuard makes sure your config agrees — before you deploy.

```bash
npx __MAIN_PACKAGE__ doctor       # friendly health report
npx __MAIN_PACKAGE__ check        # CI-friendly, exit 1 on errors
```

or add it to your project:

```bash
npm install --save-dev __MAIN_PACKAGE__
```

```json
{ "scripts": { "envcheck": "envguard check" } }
```

This package contains a small launcher; the native binary for your platform
comes from one of the `__PLATFORM_PREFIX__-*` packages, which npm installs
automatically as an optional dependency. Nothing is downloaded at install time
and EnvGuard never uses the network.

Documentation: https://github.com/ardamoustafa1/envguard
