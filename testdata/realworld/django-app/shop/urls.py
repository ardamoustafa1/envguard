from django.urls import path

from . import views

urlpatterns = [
    path("healthz/", views.healthz, name="healthz"),
    path("products/", views.product_list, name="product-list"),
    path("webhooks/payments/", views.payment_webhook, name="payment-webhook"),
]
