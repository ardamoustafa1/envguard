<?php

namespace App\Providers;

use Illuminate\Support\Env;
use Illuminate\Support\Facades\URL;
use Illuminate\Support\ServiceProvider;

class AppServiceProvider extends ServiceProvider
{
    public function register(): void
    {
        //
    }

    public function boot(): void
    {
        if (Env::get('APP_ENV') === 'production') {
            URL::forceScheme('https');
        }
    }
}
