import os

import redis

QUEUE_URL = os.environ["QUEUE_URL"]
CONCURRENCY = int(os.getenv("WORKER_CONCURRENCY", "4"))


def main():
    queue = redis.from_url(QUEUE_URL)
    print("worker started", CONCURRENCY, queue)


if __name__ == "__main__":
    main()
