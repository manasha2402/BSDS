import json
import time
from datetime import datetime, timezone
from locust import FastHttpUser, task

class ShoppingCartUser(FastHttpUser):
    def on_start(self):
        self.cart_ids = []
        self.results = []
        self.step = 0

    def record_result(self, operation, response_time_ms, success, status_code):
        self.results.append({
            "operation": operation,
            "response_time": round(response_time_ms, 2),
            "success": success,
            "status_code": status_code,
            "timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        })

    @task
    def run_sequence(self):
        if self.step >= 150:
            with open("mysql_test_results.json", "w", encoding="utf-8") as f:
                json.dump(self.results, f, indent=2)
            self.environment.runner.quit()
            return

        # 50 create cart
        if self.step < 50:
            i = self.step

            start = time.perf_counter()
            response = self.client.post(
                "/shopping-carts",
                json={"customer_id": i + 1},
                name="POST /shopping-carts"
            )
            response_time_ms = (time.perf_counter() - start) * 1000

            success = response.status_code == 201
            if success:
                body = response.json()
                cart_id = body.get("shopping_cart_id")
                if cart_id is not None:
                    self.cart_ids.append(cart_id)

            self.record_result(
                "create_cart",
                response_time_ms,
                success,
                response.status_code
            )

        # 50 add items
        elif self.step < 100:
            idx = self.step - 50
            cart_id = self.cart_ids[idx]

            start = time.perf_counter()
            response = self.client.post(
                f"/shopping-carts/{cart_id}/items",
                json={"product_id": 12345, "quantity": 1},
                name="POST /shopping-carts/{id}/items"
            )
            response_time_ms = (time.perf_counter() - start) * 1000

            self.record_result(
                "add_items",
                response_time_ms,
                response.status_code == 204,
                response.status_code
            )

        # 50 get cart
        else:
            idx = self.step - 100
            cart_id = self.cart_ids[idx]

            start = time.perf_counter()
            response = self.client.get(
                f"/shopping-carts/{cart_id}",
                name="GET /shopping-carts/{id}"
            )
            response_time_ms = (time.perf_counter() - start) * 1000

            self.record_result(
                "get_cart",
                response_time_ms,
                response.status_code == 200,
                response.status_code
            )

        self.step += 1
        