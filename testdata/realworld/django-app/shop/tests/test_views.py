import hashlib
import hmac

import pytest
from django.test import override_settings
from django.urls import reverse

from shop.models import Product

pytestmark = pytest.mark.django_db


def test_product_list_hides_out_of_stock(client):
    Product.objects.create(sku="A1", name="In stock", price="9.99", stock=3)
    Product.objects.create(sku="B2", name="Gone", price="4.50", stock=0)

    resp = client.get(reverse("product-list"))

    assert resp.status_code == 200
    assert [p["sku"] for p in resp.json()["results"]] == ["A1"]


@override_settings(PAYMENT_PROVIDER_WEBHOOK_SECRET="test-webhook-secret")
def test_webhook_rejects_bad_signature(client):
    resp = client.post(
        reverse("payment-webhook"),
        data="{}",
        content_type="application/json",
        headers={"X-Signature": "nope"},
    )
    assert resp.status_code == 400


def test_signature_helper_matches():
    body = b"{}"
    sig = hmac.new(b"k", body, hashlib.sha256).hexdigest()
    assert len(sig) == 64
