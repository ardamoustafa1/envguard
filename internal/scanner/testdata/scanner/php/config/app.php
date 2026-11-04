<p>Docs: getenv('FAKE_IN_HTML')</p>
<?php
// getenv('FAKE_IN_COMMENT')
return [
    'key' => env('REAL_APP_KEY'),
    'debug' => env('REAL_APP_DEBUG', false),
    'db' => getenv('REAL_DB_HOST') ?: 'localhost',
    'mail' => $_ENV['REAL_MAIL_HOST'] ?? 'smtp',
    'host' => $_SERVER['HTTP_HOST'],
    'help' => "call getenv('FAKE_IN_STRING')",
    'dyn' => getenv($name),
];
