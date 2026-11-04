"""Settings module.

Reads os.environ["FAKE_IN_DOCSTRING"] at import time.
"""
import os
from os import environ

# os.getenv("FAKE_IN_COMMENT")
DATABASE_URL = os.environ["REAL_DATABASE_URL"]
DEBUG = os.environ.get("REAL_DEBUG", "false") == "true"
SECRET = os.getenv("REAL_SECRET_KEY")
CACHE = environ.get("REAL_CACHE_URL")
HELP = "export os.getenv('FAKE_IN_STRING')"
name = "REAL_DYNAMIC"
VALUE = os.getenv(name)
os.environ["FAKE_ASSIGNED"] = "1"
