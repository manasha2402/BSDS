import random
from locust import HttpUser, task, between


class ProductUser(HttpUser):
    wait_time = between(1, 3)

    @task(5)
    def get_product(self):
        pid = random.randint(1, 10)
        self.client.get(f"/products/{pid}", name="/products/[id]")

    @task(1)
    def create_product(self):
        pid = random.randint(10000, 9999999)
        with self.client.post("/products", json={
            "product_id": pid,
            "sku": f"SKU-{pid}",
            "manufacturer": f"Mfg-{pid}",
            "category_id": random.randint(1, 50),
            "weight": random.randint(100, 5000),
            "some_other_id": random.randint(1, 100),
        }, catch_response=True) as resp:
            if resp.status_code in (201, 400):
                resp.success()

    @task(2)
    def update_product(self):
        pid = random.randint(1, 10)
        self.client.post(f"/products/{pid}/details", json={
            "product_id": pid,
            "sku": f"SKU-UPDATED-{pid}",
            "manufacturer": f"Updated-Mfg-{pid}",
            "category_id": random.randint(1, 50),
            "weight": random.randint(100, 5000),
            "some_other_id": random.randint(1, 100),
        }, name="/products/[id]/details")

    @task(1)
    def get_nonexistent(self):
        with self.client.get("/products/99999", name="/products/[id] (404)",
                             catch_response=True) as resp:
            if resp.status_code == 404:
                resp.success()