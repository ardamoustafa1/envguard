# Shop

A small Django 5 shop backend with Celery workers.

## Local development

```bash
python -m venv .venv && . .venv/bin/activate
pip install -r requirements/local.txt
cp .env.example .env
python manage.py migrate
python manage.py runserver
```

Or with Docker: `docker compose up`.

## Configuration

Settings live in `config/settings/` and are driven by environment variables
(django-environ). `.env.example` lists every variable. Under Docker, variables
come from `.envs/.local/*` and `.envs/.production/*`.

| Variable | Used in | Notes |
| --- | --- | --- |
| `DJANGO_SECRET_KEY` | settings | required in production |
| `DATABASE_URL` | settings | `postgres://user:pass@host:5432/db` |
| `REDIS_URL` | settings | cache, Celery default broker |
| `PAYMENT_WEBHOOK_SECRET` | settings | HMAC key for payment webhooks |
| `SENTRY_DSN` | production | optional |

## Tests

```bash
pytest
```
