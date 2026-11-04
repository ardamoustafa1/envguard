FROM python:3.12-slim-bookworm

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends libpq-dev build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY requirements ./requirements
ARG BUILD_ENVIRONMENT=production
RUN pip install --no-cache-dir -r requirements/${BUILD_ENVIRONMENT}.txt

COPY compose/production/django/entrypoint /entrypoint
COPY compose/production/django/start /start
RUN sed -i 's/\r$//g' /entrypoint /start && chmod +x /entrypoint /start

COPY . /app

ENTRYPOINT ["/entrypoint"]
CMD ["/start"]
