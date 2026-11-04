from django.contrib import admin

from .models import Order, OrderItem, Product


class OrderItemInline(admin.TabularInline):
    model = OrderItem
    extra = 0


@admin.register(Product)
class ProductAdmin(admin.ModelAdmin):
    list_display = ("sku", "name", "price", "stock")
    search_fields = ("sku", "name")


@admin.register(Order)
class OrderAdmin(admin.ModelAdmin):
    list_display = ("id", "user", "status", "created")
    inlines = [OrderItemInline]
