import { OpenAPIHono, createRoute, z } from "@hono/zod-openapi";
import { eq } from "drizzle-orm";
import { db } from "../db/index";
import { restaurantsTable } from "../db/schema";
import {
  createRestaurantDto,
  updateRestaurantDto,
} from "../dto/restaurant.dto";
import { customValidationHook } from "../lib/hooks";

export const restaurantsRouter = new OpenAPIHono({
  defaultHook: customValidationHook,
});

// ── Reusable schemas ────────────────────────────────────────────────

const ErrorSchema = z
  .object({ error: z.string() })
  .openapi("RestaurantErrorResponse");

const RestaurantSchema = z
  .object({
    id: z.number(),
    owner_id: z.number(),
    name: z.string(),
    address: z.string(),
    status: z.string(),
    image_url: z.string(),
    createdAt: z.string(),
    updatedAt: z.string(),
  })
  .openapi("Restaurant");

// ── GET /restaurants ────────────────────────────────────────────────

const listRoute = createRoute({
  method: "get",
  path: "/",
  tags: ["Restaurants"],
  summary: "List all restaurants",
  responses: {
    200: {
      content: {
        "application/json": {
          schema: z.object({ data: z.array(RestaurantSchema) }),
        },
      },
      description: "All restaurants",
    },
  },
});

restaurantsRouter.openapi(listRoute, async (c) => {
  const all = await db.select().from(restaurantsTable);
  return c.json({ data: all } as any, 200);
});

// ── GET /restaurants/:id ────────────────────────────────────────────

const getByIdRoute = createRoute({
  method: "get",
  path: "/{id}",
  tags: ["Restaurants"],
  summary: "Get a restaurant by ID",
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
          schema: z.object({ data: RestaurantSchema }),
        },
      },
      description: "Restaurant found",
    },
    400: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid ID",
    },
    404: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Restaurant not found",
    },
  },
});

restaurantsRouter.openapi(getByIdRoute, async (c) => {
  const { id } = c.req.valid("param");
  const numId = parseInt(id, 10);
  if (isNaN(numId) || numId <= 0) {
    return c.json({ error: "Invalid restaurant ID" }, 400);
  }

  const [restaurant] = await db
    .select()
    .from(restaurantsTable)
    .where(eq(restaurantsTable.id, numId));

  if (!restaurant) return c.json({ error: "Restaurant not found" }, 404);
  return c.json({ data: restaurant } as any, 200);
});

// ── POST /restaurants ───────────────────────────────────────────────

const createRestaurantRoute = createRoute({
  method: "post",
  path: "/",
  tags: ["Restaurants"],
  summary: "Create a restaurant",
  request: {
    body: {
      content: { "application/json": { schema: createRestaurantDto } },
      required: true,
      description: "Restaurant payload",
    },
  },
  responses: {
    201: {
      content: {
        "application/json": {
          schema: z.object({ data: RestaurantSchema }),
        },
      },
      description: "Restaurant created",
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

restaurantsRouter.openapi(createRestaurantRoute, async (c) => {
  const body = c.req.valid("json");
  try {
    const [result] = await db.insert(restaurantsTable).values(body).returning();
    return c.json({ data: result } as any, 201);
  } catch (error) {
    console.error("Failed to create restaurant:", error);
    return c.json({ error: "Failed to create restaurant" }, 500);
  }
});

// ── PATCH /restaurants/:id ──────────────────────────────────────────

const updateRoute = createRoute({
  method: "patch",
  path: "/{id}",
  tags: ["Restaurants"],
  summary: "Update a restaurant",
  request: {
    params: z.object({
      id: z
        .string()
        .regex(/^\d+$/, "ID must be a numeric string")
        .openapi({ param: { name: "id", in: "path" }, example: "1" }),
    }),
    body: {
      content: { "application/json": { schema: updateRestaurantDto } },
      required: true,
      description: "Fields to update",
    },
  },
  responses: {
    200: {
      content: {
        "application/json": {
          schema: z.object({ data: RestaurantSchema }),
        },
      },
      description: "Restaurant updated",
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

restaurantsRouter.openapi(updateRoute, async (c) => {
  const { id } = c.req.valid("param");
  const numId = parseInt(id, 10);
  if (isNaN(numId) || numId <= 0) {
    return c.json({ error: "Invalid restaurant ID" }, 400);
  }

  const body = c.req.valid("json");
  try {
    const [updated] = await db
      .update(restaurantsTable)
      .set({ ...body, updatedAt: new Date() })
      .where(eq(restaurantsTable.id, numId))
      .returning();

    if (!updated) return c.json({ error: "Restaurant not found" }, 404);
    return c.json({ data: updated } as any, 200);
  } catch (error) {
    console.error("Failed to update restaurant:", error);
    return c.json({ error: "Failed to update restaurant" }, 500);
  }
});
