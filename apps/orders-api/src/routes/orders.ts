import { OpenAPIHono, createRoute, z } from "@hono/zod-openapi";
import { eq } from "drizzle-orm";
import { db } from "../db/index";
import { ordersTable, orderItemsTable } from "../db/schema";
import { createOrderDto, updateOrderStatusDto } from "../dto/order.dto";
import { publishEvent } from "../lib/rabbitmq";
import { customValidationHook } from "../lib/hooks";

export const ordersRouter = new OpenAPIHono({
  defaultHook: customValidationHook,
});

// ── Reusable schemas ────────────────────────────────────────────────

const ErrorSchema = z
  .object({
    error: z.string(),
  })
  .openapi("ErrorResponse");

const OrderSchema = z
  .object({
    id: z.number(),
    user_id: z.number(),
    restaurant_id: z.number(),
    status: z.string(),
    total_amount: z.number(),
    delivery_address: z.string(),
    createdAt: z.string(),
    updatedAt: z.string(),
  })
  .openapi("Order");

const OrderItemSchema = z
  .object({
    id: z.number(),
    order_id: z.number(),
    menu_item_id: z.number(),
    quantity: z.number(),
    unit_price_at_purchase: z.number(),
    createdAt: z.string(),
    updatedAt: z.string(),
  })
  .openapi("OrderItem");

const OrderWithItemsSchema = OrderSchema.extend({
  orderItems: z.array(OrderItemSchema),
}).openapi("OrderWithItems");

// ── GET /orders ─────────────────────────────────────────────────────

const getOrdersRoute = createRoute({
  method: "get",
  path: "/",
  tags: ["Orders"],
  summary: "Retrieve all orders",
  responses: {
    200: {
      content: {
        "application/json": {
          schema: z.object({
            data: z.array(OrderSchema),
          }),
        },
      },
      description: "Retrieve all orders",
    },
  },
});

ordersRouter.openapi(getOrdersRoute, async (c) => {
  const allOrders = await db.select().from(ordersTable);
  return c.json({ data: allOrders } as any, 200);
});

// ── GET /orders/:id ─────────────────────────────────────────────────

const getOrderByIdRoute = createRoute({
  method: "get",
  path: "/{id}",
  tags: ["Orders"],
  summary: "Retrieve an order by ID with its items",
  request: {
    params: z.object({
      id: z
        .string()
        .regex(/^\d+$/, "ID must be a numeric string")
        .openapi({ param: { name: "id", in: "path" }, example: "1" }),
    }),
  },
  responses: {
    200: {
      content: {
        "application/json": {
          schema: z.object({ data: OrderWithItemsSchema }),
        },
      },
      description: "Order found",
    },
    400: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid order ID",
    },
    404: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Order not found",
    },
  },
});

ordersRouter.openapi(getOrderByIdRoute, async (c) => {
  const { id } = c.req.valid("param");
  const orderId = parseInt(id, 10);

  if (isNaN(orderId) || orderId <= 0) {
    return c.json({ error: "Invalid order ID" }, 400);
  }

  const order = await db.query.ordersTable.findFirst({
    where: eq(ordersTable.id, orderId),
    with: {
      orderItems: true,
    },
  });

  if (!order) return c.json({ error: "Order not found" }, 404);
  return c.json({ data: order } as any, 200);
});

// ── POST /orders ────────────────────────────────────────────────────

const createOrderRoute = createRoute({
  method: "post",
  path: "/",
  tags: ["Orders"],
  summary: "Create a new order with items",
  request: {
    body: {
      content: {
        "application/json": {
          schema: createOrderDto,
        },
      },
      required: true,
      description: "Order payload with items",
    },
  },
  responses: {
    201: {
      content: {
        "application/json": {
          schema: z.object({ data: OrderSchema }),
        },
      },
      description: "Order created successfully",
    },
    400: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid request body",
    },
    500: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Failed to create order",
    },
  },
});

ordersRouter.openapi(createOrderRoute, async (c) => {
  const body = c.req.valid("json");

  try {
    const result = await db.transaction(async (tx) => {
      const [newOrder] = await tx
        .insert(ordersTable)
        .values({
          user_id: body.user_id,
          restaurant_id: body.restaurant_id,
          total_amount: body.total_amount,
          delivery_address: body.delivery_address,
          status: "PENDING",
        })
        .returning();

      const orderItemsToInsert = body.items.map((item) => ({
        order_id: newOrder.id,
        menu_item_id: item.menu_item_id,
        quantity: item.quantity,
        unit_price_at_purchase: item.unit_price_at_purchase,
      }));

      await tx.insert(orderItemsTable).values(orderItemsToInsert);

      return newOrder;
    });

    return c.json({ data: result } as any, 201);
  } catch (error) {
    console.error("Failed to create order:", error);
    return c.json({ error: "Failed to create order" }, 500);
  }
});

// ── PATCH /orders/:id/status ────────────────────────────────────────

const updateOrderStatusRoute = createRoute({
  method: "patch",
  path: "/{id}/status",
  tags: ["Orders"],
  summary: "Update an order's status",
  description:
    "Updates the status of an existing order. When status is set to READY_FOR_DELIVERY, an event is published to RabbitMQ for the delivery service.",
  request: {
    params: z.object({
      id: z
        .string()
        .regex(/^\d+$/, "ID must be a numeric string")
        .openapi({ param: { name: "id", in: "path" }, example: "1" }),
    }),
    body: {
      content: {
        "application/json": {
          schema: updateOrderStatusDto,
        },
      },
      required: true,
      description: "New order status",
    },
  },
  responses: {
    200: {
      content: {
        "application/json": {
          schema: z.object({ data: OrderSchema }),
        },
      },
      description: "Order status updated",
    },
    400: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid order ID",
    },
    404: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Order not found",
    },
    500: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Failed to update order status",
    },
  },
});

ordersRouter.openapi(updateOrderStatusRoute, async (c) => {
  const { id } = c.req.valid("param");
  const orderId = parseInt(id, 10);

  if (isNaN(orderId) || orderId <= 0) {
    return c.json({ error: "Invalid order ID" }, 400);
  }

  const { status } = c.req.valid("json");

  try {
    const [updatedOrder] = await db
      .update(ordersTable)
      .set({ status, updatedAt: new Date() })
      .where(eq(ordersTable.id, orderId))
      .returning();

    if (!updatedOrder) {
      return c.json({ error: "Order not found" }, 404);
    }

    // Publish event if status is READY_FOR_DELIVERY
    if (status === "READY_FOR_DELIVERY") {
      const eventPayload = {
        orderId: updatedOrder.id,
        restaurantId: updatedOrder.restaurant_id,
        deliveryAddress: updatedOrder.delivery_address,
        timestamp: new Date().toISOString(),
      };

      // Using 'order.ready' as the routing key
      await publishEvent("order.ready", eventPayload);
      console.log(
        `Published order.ready event for Order ID: ${updatedOrder.id}`,
      );
    }

    return c.json({ data: updatedOrder } as any, 200);
  } catch (error) {
    console.error("Failed to update order status:", error);
    return c.json({ error: "Failed to update order status" }, 500);
  }
});
