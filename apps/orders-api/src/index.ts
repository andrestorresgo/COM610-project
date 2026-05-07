import { Hono } from "hono";
import { drizzle } from "drizzle-orm/node-postgres";
import { usersTable } from "./db/schema";

const db = drizzle(process.env.DATABASE_URL!);
const app = new Hono();

app.get("/", async (c) => {
  return c.json({
    data: await db.select().from(usersTable),
  });
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
