# Journal

A small publishing platform built with Laravel 11.

## Getting started

```bash
cp .env.example .env
composer install
php artisan key:generate
php artisan migrate --seed
npm install && npm run dev
```

With Docker (Laravel Sail):

```bash
./vendor/bin/sail up -d
./vendor/bin/sail artisan migrate --seed
```

## Configuration

All configuration lives in `.env`; see `.env.example` for every supported key.
Do not call `env()` outside of the `config/` directory: read values through `config()`.

## Testing

```bash
php artisan test
```

`phpunit.xml` pins the testing environment (in-memory SQLite, array cache and sessions).
