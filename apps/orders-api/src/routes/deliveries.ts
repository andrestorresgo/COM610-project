import { OpenAPIHono, createRoute, z } from "@hono/zod-openapi";
import { eq } from "drizzle-orm";
import { db } from "../db/index";
import { deliveriesTable } from "../db/schema";
import {
  createDeliveryDto,
  updateDeliveryStatusDto,
} from "../dto/delivery.dto";
import { customValidationHook } from "../lib/hooks";

export const deliveriesRouter = new OpenAPIHono({
  defaultHook: customValidationHook,
});

// ── Reusable schemas ────────────────────────────────────────────────

const ErrorSchema = z
  .object({ error: z.string() })
  .openapi("DeliveryErrorResponse");

const DeliverySchema = z
  .object({
    id: z.number(),
    order_id: z.number(),
    courier_id: z.number(),
    status: z.string(),
    picked_up_at: z.string().nullable(),
    delivered_at: z.string().nullable(),
    createdAt: z.string(),
    updatedAt: z.string(),
  })
  .openapi("Delivery");

// ── GET /deliveries ─────────────────────────────────────────────────

const listRoute = createRoute({
  method: "get",
  path: "/",
  tags: ["Deliveries"],
  summary: "List all deliveries",
  responses: {
    200: {
      content: {
        "application/json": {
          schema: z.object({ data: z.array(DeliverySchema) }),
        },
      },
      description: "All deliveries",
    },
  },
});

deliveriesRouter.openapi(listRoute, async (c) => {
  const all = await db.select().from(deliveriesTable);
  return c.json({ data: all } as any, 200);
});

// ── GET /deliveries/:id ─────────────────────────────────────────────

const getByIdRoute = createRoute({
  method: "get",
  path: "/{id}",
  tags: ["Deliveries"],
  summary: "Get a delivery by ID",
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
          schema: z.object({ data: DeliverySchema }),
        },
      },
      description: "Delivery found",
    },
    400: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid ID",
    },
    404: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Delivery not found",
    },
  },
});

deliveriesRouter.openapi(getByIdRoute, async (c) => {
  const { id } = c.req.valid("param");
  const numId = parseInt(id, 10);
  if (isNaN(numId) || numId <= 0) {
    return c.json({ error: "Invalid delivery ID" }, 400);
  }

  const [delivery] = await db
    .select()
    .from(deliveriesTable)
    .where(eq(deliveriesTable.id, numId));

  if (!delivery) return c.json({ error: "Delivery not found" }, 404);
  return c.json({ data: delivery } as any, 200);
});

// ── POST /deliveries ────────────────────────────────────────────────

const createDeliveryRoute = createRoute({
  method: "post",
  path: "/",
  tags: ["Deliveries"],
  summary: "Create a delivery",
  request: {
    body: {
      content: { "application/json": { schema: createDeliveryDto } },
      required: true,
      description: "Delivery payload",
    },
  },
  responses: {
    201: {
      content: {
        "application/json": {
          schema: z.object({ data: DeliverySchema }),
        },
      },
      description: "Delivery created",
    },
    400: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid body",
    },
    500: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Creation failed",
    },
  },
});

deliveriesRouter.openapi(createDeliveryRoute, async (c) => {
  const body = c.req.valid("json");
  try {
    const [result] = await db.insert(deliveriesTable).values(body).returning();
    return c.json({ data: result } as any, 201);
  } catch (error) {
    console.error("Failed to create delivery:", error);
    return c.json({ error: "Failed to create delivery" }, 500);
  }
});

// ── PATCH /deliveries/:id/status ────────────────────────────────────

const updateStatusRoute = createRoute({
  method: "patch",
  path: "/{id}/status",
  tags: ["Deliveries"],
  summary: "Update delivery status",
  request: {
    params: z.object({
      id: z
        .string()
        .regex(/^\d+$/, "ID must be a numeric string")
        .openapi({ param: { name: "id", in: "path" }, example: "1" }),
    }),
    body: {
      content: { "application/json": { schema: updateDeliveryStatusDto } },
      required: true,
      description: "New status",
    },
  },
  responses: {
    200: {
      content: {
        "application/json": {
          schema: z.object({ data: DeliverySchema }),
        },
      },
      description: "Delivery status updated",
    },
    400: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid ID",
    },
    404: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Not found",
    },
    500: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Update failed",
    },
  },
});

deliveriesRouter.openapi(updateStatusRoute, async (c) => {
  const { id } = c.req.valid("param");
  const numId = parseInt(id, 10);
  if (isNaN(numId) || numId <= 0) {
    return c.json({ error: "Invalid delivery ID" }, 400);
  }

  const { status } = c.req.valid("json");

  try {
    const updateData: any = { status, updatedAt: new Date() };

    if (status === "PICKED_UP") {
      updateData.picked_up_at = new Date();
    } else if (status === "DELIVERED") {
      updateData.delivered_at = new Date();
    }

    const [updated] = await db
      .update(deliveriesTable)
      .set(updateData)
      .where(eq(deliveriesTable.id, numId))
      .returning();

    if (!updated) return c.json({ error: "Delivery not found" }, 404);
    return c.json({ data: updated } as any, 200);
  } catch (error) {
    console.error("Failed to update delivery status:", error);
    return c.json({ error: "Failed to update delivery status" }, 500);
  }
});
