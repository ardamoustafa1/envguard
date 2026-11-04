# Ledgerly

A JSON API for invoicing, built with Rails 7.1, PostgreSQL and Sidekiq.

## Setup

```bash
bin/setup
```

This copies `.env.example` to `.env` (loaded by `dotenv-rails` in development and test), prepares the
database and starts the server.

## Credentials

Secrets are stored in `config/credentials.yml.enc`. The decryption key lives in `config/master.key`
(never committed) or in the `RAILS_MASTER_KEY` environment variable in production.

## Running the tests

```bash
bundle exec rspec
```

## Deployment

The `Dockerfile` is the stock Rails 7.1 production image. `Procfile` is provided for Heroku-style hosts.
Set `DATABASE_URL`, `REDIS_URL` and `RAILS_MASTER_KEY` in the host's configuration.
