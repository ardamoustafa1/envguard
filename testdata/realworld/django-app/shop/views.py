import hashlib
import hmac
import json

from django.conf import settings
from django.http import HttpResponse, HttpResponseBadRequest, JsonResponse
from django.views.decorators.csrf import csrf_exempt
from django.views.decorators.http import require_GET, require_POST

from .models import Order, Product
from .tasks import send_order_confirmation


@require_GET
def product_list(request):
    products = Product.objects.filter(stock__gt=0).values("sku", "name", "price")
    return JsonResponse({"results": list(products)})


@require_GET
def healthz(request):
    return HttpResponse("ok")


@csrf_exempt
@require_POST
def payment_webhook(request):
    signature = request.headers.get("X-Signature", "")
    expected = hmac.new(
        settings.PAYMENT_PROVIDER_WEBHOOK_SECRET.encode(),
        request.body,
        hashlib.sha256,
    ).hexdigest()
    if not hmac.compare_digest(signature, expected):
        return HttpResponseBadRequest("bad signature")

    payload = json.loads(request.body)
    order = Order.objects.get(pk=payload["order_id"])
    order.status = Order.Status.PAID
    order.save(update_fields=["status"])
    send_order_confirmation.delay(order.pk)
    return JsonResponse({"ok": True})
