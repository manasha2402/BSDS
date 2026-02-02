from locust import FastHttpUser, task, between

class QuickstartUser(FastHttpUser):
    wait_time = between(1, 2)
    
    @task(3)
    def get_albums(self):
        self.client.get("/albums")
    
    @task(1)
    def post_album(self):
        self.client.post("/albums", json={"title": "test", "artist": "test"})