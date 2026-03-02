
from locust import FastHttpUser, constant, task, between

class ProductSearchUser(FastHttpUser):
    wait_time = constant(0) 

    search_terms = ["electronics", "books", "home", "clothing", "sports",
                    "alpha", "beta", "gamma", "delta", "epsilon"]

    @task
    def search_products(self):
        for term in self.search_terms:
            self.client.get(f"/products/search?q={term}")

    @task
    def health_check(self):
        self.client.get("/health")