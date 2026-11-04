from celery import shared_task
from django.core.mail import send_mail

from .models import Order


@shared_task
def send_order_confirmation(order_id: int) -> None:
    order = Order.objects.select_related("user").get(pk=order_id)
    send_mail(
        subject=f"Order #{order.pk} confirmed",
        message="Thanks for your purchase.",
        from_email=None,
        recipient_list=[order.user.email],
    )
