import pytest


@pytest.fixture(autouse=True)
def _settings_module(monkeypatch):
    monkeypatch.setenv("DJANGO_SETTINGS_MODULE", "config.settings.test")
