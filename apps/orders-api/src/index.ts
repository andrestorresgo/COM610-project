import { Hono } from "hono";

const app = new Hono();

app.get("/", (c) => {
  return c.text("Hello from Orders API!");
});

app.get("/health", (c) => {
  return c.json({
    status: "healthy",
    service: "orders-api",
    timestamp: new Date().toISOString(),
  });
});

export default {
  port: process.env.PORT,
  fetch: app.fetch,
};
