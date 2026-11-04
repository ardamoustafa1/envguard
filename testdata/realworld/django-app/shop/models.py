from decimal import Decimal

from django.conf import settings
from django.db import models


class Product(models.Model):
    sku = models.CharField(max_length=32, unique=True)
    name = models.CharField(max_length=200)
    price = models.DecimalField(max_digits=10, decimal_places=2)
    stock = models.PositiveIntegerField(default=0)
    created = models.DateTimeField(auto_now_add=True)

    class Meta:
        ordering = ["name"]

    def __str__(self):
        return self.name


class Order(models.Model):
    class Status(models.TextChoices):
        PENDING = "pending"
        PAID = "paid"
        SHIPPED = "shipped"

    user = models.ForeignKey(settings.AUTH_USER_MODEL, on_delete=models.CASCADE)
    status = models.CharField(max_length=16, choices=Status.choices, default=Status.PENDING)
    created = models.DateTimeField(auto_now_add=True)

    def total(self) -> Decimal:
        return sum((i.price * i.quantity for i in self.items.all()), Decimal("0"))


class OrderItem(models.Model):
    order = models.ForeignKey(Order, related_name="items", on_delete=models.CASCADE)
    product = models.ForeignKey(Product, on_delete=models.PROTECT)
    quantity = models.PositiveIntegerField(default=1)
    price = models.DecimalField(max_digits=10, decimal_places=2)
