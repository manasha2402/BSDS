from locust import HttpUser, task, between
import json
import random

class OrderUser(HttpUser):
    wait_time = between(0.1, 0.5)  # 100-500ms between requests

    @task
    def place_order(self):
        payload = {
            "customer_id": random.randint(1, 1000),
            "items": [
                {"name": "shoe", "quantity": 1, "price": 49.99}
            ]
        }
        self.client.post("/orders/async", json=payload)