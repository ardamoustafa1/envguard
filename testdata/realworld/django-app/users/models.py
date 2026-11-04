from django.contrib.auth.models import AbstractUser
from django.db import models


class User(AbstractUser):
    """Default user for the shop."""

    phone = models.CharField(max_length=32, blank=True)

    def __str__(self):
        return self.get_username()
