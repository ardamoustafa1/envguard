from unittest import mock

from shop import inventory


def test_fetch_stock(monkeypatch):
    monkeypatch.setenv("INVENTORY_API_URL", "http://inventory.test")
    fake = mock.Mock()
    fake.json.return_value = {"quantity": 7}
    with mock.patch("shop.inventory.requests.get", return_value=fake):
        assert inventory.fetch_stock("A1") == 7
