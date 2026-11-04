"""With these settings, tests run faster."""
from .base import *  # noqa
from .base import env

SECRET_KEY = env("DJANGO_SECRET_KEY", default="test-only-secret-key")
TEST_RUNNER = "django.test.runner.DiscoverRunner"
PASSWORD_HASHERS = ["django.contrib.auth.hashers.MD5PasswordHasher"]
EMAIL_BACKEND = "django.core.mail.backends.locmem.EmailBackend"
CELERY_TASK_ALWAYS_EAGER = True
