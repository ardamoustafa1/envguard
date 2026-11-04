import environ

env = environ.Env()
ALLOWED = env.list("REAL_ALLOWED_HOSTS", default=["localhost"])
SENTRY = env("REAL_SENTRY_DSN")
