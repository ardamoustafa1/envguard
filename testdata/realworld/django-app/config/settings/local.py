from .base import *  # noqa
from .base import env

# GENERAL
# ------------------------------------------------------------------------------
DEBUG = True
# https://docs.djangoproject.com/en/dev/ref/settings/#secret-key
SECRET_KEY = env(
    "DJANGO_SECRET_KEY",
    default="dev-only-secret-key-do-not-use-in-production",
)
ALLOWED_HOSTS = ["localhost", "0.0.0.0", "127.0.0.1"]

# EMAIL
# ------------------------------------------------------------------------------
EMAIL_BACKEND = "django.core.mail.backends.console.EmailBackend"

# django-debug-toolbar
# ------------------------------------------------------------------------------
INSTALLED_APPS += ["debug_toolbar"]  # noqa F405
MIDDLEWARE.insert(0, "debug_toolbar.middleware.DebugToolbarMiddleware")  # noqa F405
INTERNAL_IPS = ["127.0.0.1", "10.0.2.2"]

# Celery
# ------------------------------------------------------------------------------
CELERY_TASK_EAGER_PROPAGATES = True
