"""Client for the warehouse inventory service."""
import os

import requests


def fetch_stock(sku: str) -> int:
    base_url = os.environ["INVENTORY_API_URL"]
    resp = requests.get(f"{base_url}/stock/{sku}", timeout=5)
    resp.raise_for_status()
    return int(resp.json()["quantity"])
