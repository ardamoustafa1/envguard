import os

import requests

PAYMENT_API_BASE = os.environ.get("PAYMENT_API_BASE", "https://api.payments.example.com")


def create_charge(order_id: int, amount_cents: int) -> str:
    token = os.getenv("PAYMENT_API_TOKEN", "")
    resp = requests.post(
        f"{PAYMENT_API_BASE}/charges",
        json={"order": order_id, "amount": amount_cents},
        headers={"Authorization": f"Bearer {token}"},
        timeout=10,
    )
    resp.raise_for_status()
    return resp.json()["id"]
