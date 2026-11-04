"""WSGI config for the shop project."""
import os
import sys
from pathlib import Path

from django.core.wsgi import get_wsgi_application

# This allows easy placement of apps within the interior directory.
BASE_DIR = Path(__file__).resolve(strict=True).parent.parent
sys.path.append(str(BASE_DIR))

# We defer to a DJANGO_SETTINGS_MODULE already in the environment.
os.environ.setdefault("DJANGO_SETTINGS_MODULE", "config.settings.production")

application = get_wsgi_application()
