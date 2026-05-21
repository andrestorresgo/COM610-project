import { OpenAPIHono } from "@hono/zod-openapi";
import { swaggerUI } from "@hono/swagger-ui";
import { ordersRouter } from "./routes/orders";
import { usersRouter } from "./routes/users";
import { restaurantsRouter } from "./routes/restaurants";
import { categoriesRouter } from "./routes/categories";
import { menuItemsRouter } from "./routes/menu-items";
import { deliveriesRouter } from "./routes/deliveries";
import { imagesRouter } from "./routes/images";
import { cors } from "hono/cors";

const app = new OpenAPIHono();

app.use("*", cors());

app.doc("/doc", {
  openapi: "3.0.0",
  info: {
    version: "1.0.0",
    title: "Orders API",
  },
});

app.get("/ui", swaggerUI({ url: "/doc" }));

app.get("/health", (c) => {
  return c.json({
    status: "healthy",
    service: "orders-api",
    timestamp: new Date().toISOString(),
  });
});

app.route("/orders", ordersRouter);
app.route("/users", usersRouter);
app.route("/restaurants", restaurantsRouter);
app.route("/categories", categoriesRouter);
app.route("/menu-items", menuItemsRouter);
app.route("/deliveries", deliveriesRouter);
app.route("/images", imagesRouter);

export default {
  port: process.env.PORT,
  fetch: app.fetch,
};
