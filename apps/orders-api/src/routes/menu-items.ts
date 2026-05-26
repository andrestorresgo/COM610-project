import { OpenAPIHono, createRoute, z } from "@hono/zod-openapi";
import { eq } from "drizzle-orm";
import { db } from "../db/index";
import { menuItemsTable } from "../db/schema";
import { createMenuItemDto, updateMenuItemDto } from "../dto/menuItem.dto";
import { customValidationHook } from "../lib/hooks";

export const menuItemsRouter = new OpenAPIHono({
  defaultHook: customValidationHook,
});

// ── Reusable schemas ────────────────────────────────────────────────

const ErrorSchema = z
  .object({ error: z.string() })
  .openapi("MenuItemErrorResponse");

const MenuItemSchema = z
  .object({
    id: z.number(),
    restaurant_id: z.number(),
    category_id: z.number(),
    name: z.string(),
    description: z.string(),
    is_available: z.boolean(),
    price: z.number(),
    image_url: z.string(),
    createdAt: z.string(),
    updatedAt: z.string(),
  })
  .openapi("MenuItem");

// ── GET /restaurants/:restaurantId/menu-items ───────────────────────

const listByRestaurantRoute = createRoute({
  method: "get",
  path: "/restaurants/{restaurantId}/menu-items",
  tags: ["MenuItems"],
  summary: "List all menu items for a restaurant",
  request: {
    params: z.object({
      restaurantId: z
        .string()
        .regex(/^\d+$/, "ID must be a numeric string")
        .openapi({ param: { name: "restaurantId", in: "path" }, example: "1" }),
    }),
  },
  responses: {
    200: {
      content: {
        "application/json": {
          schema: z.object({ data: z.array(MenuItemSchema) }),
        },
      },
      description: "Menu items for restaurant",
    },
    400: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid restaurant ID",
    },
  },
});

menuItemsRouter.openapi(listByRestaurantRoute, async (c) => {
  const { restaurantId } = c.req.valid("param");
  const rId = parseInt(restaurantId, 10);
  if (isNaN(rId) || rId <= 0) {
    return c.json({ error: "Invalid restaurant ID" }, 400);
  }

  const all = await db
    .select()
    .from(menuItemsTable)
    .where(eq(menuItemsTable.restaurant_id, rId));

  return c.json({ data: all } as any, 200);
});

// ── GET /categories/:categoryId/menu-items ──────────────────────────

const listByCategoryRoute = createRoute({
  method: "get",
  path: "/categories/{categoryId}/menu-items",
  tags: ["MenuItems"],
  summary: "List all menu items for a category",
  request: {
    params: z.object({
      categoryId: z
        .string()
        .regex(/^\d+$/, "ID must be a numeric string")
        .openapi({ param: { name: "categoryId", in: "path" }, example: "1" }),
    }),
  },
  responses: {
    200: {
      content: {
        "application/json": {
          schema: z.object({ data: z.array(MenuItemSchema) }),
        },
      },
      description: "Menu items for category",
    },
    400: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid category ID",
    },
  },
});

menuItemsRouter.openapi(listByCategoryRoute, async (c) => {
  const { categoryId } = c.req.valid("param");
  const cId = parseInt(categoryId, 10);
  if (isNaN(cId) || cId <= 0) {
    return c.json({ error: "Invalid category ID" }, 400);
  }

  const all = await db
    .select()
    .from(menuItemsTable)
    .where(eq(menuItemsTable.category_id, cId));

  return c.json({ data: all } as any, 200);
});

// ── POST /menu-items ────────────────────────────────────────────────

const createMenuItemRoute = createRoute({
  method: "post",
  path: "/",
  tags: ["MenuItems"],
  summary: "Create a menu item",
  request: {
    body: {
      content: { "application/json": { schema: createMenuItemDto } },
      required: true,
      description: "Menu item payload",
    },
  },
  responses: {
    201: {
      content: {
        "application/json": {
          schema: z.object({ data: MenuItemSchema }),
        },
      },
      description: "Menu item created",
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

menuItemsRouter.openapi(createMenuItemRoute, async (c) => {
  const body = c.req.valid("json");
  try {
    const [result] = await db
      .insert(menuItemsTable)
      .values({
        ...body,
        image_url: body.image_url ?? "",
      })
      .returning();
    return c.json({ data: result } as any, 201);
  } catch (error) {
    console.error("Failed to create menu item:", error);
    return c.json({ error: "Failed to create menu item" }, 500);
  }
});

// ── PATCH /menu-items/:id ───────────────────────────────────────────

const updateRoute = createRoute({
  method: "patch",
  path: "/{id}",
  tags: ["MenuItems"],
  summary: "Update a menu item",
  request: {
    params: z.object({
      id: z
        .string()
        .regex(/^\d+$/, "ID must be a numeric string")
        .openapi({ param: { name: "id", in: "path" }, example: "1" }),
    }),
    body: {
      content: { "application/json": { schema: updateMenuItemDto } },
      required: true,
      description: "Fields to update",
    },
  },
  responses: {
    200: {
      content: {
        "application/json": {
          schema: z.object({ data: MenuItemSchema }),
        },
      },
      description: "Menu item updated",
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

menuItemsRouter.openapi(updateRoute, async (c) => {
  const { id } = c.req.valid("param");
  const numId = parseInt(id, 10);
  if (isNaN(numId) || numId <= 0) {
    return c.json({ error: "Invalid menu item ID" }, 400);
  }

  const body = c.req.valid("json");
  try {
    const [updated] = await db
      .update(menuItemsTable)
      .set({ ...body, updatedAt: new Date() })
      .where(eq(menuItemsTable.id, numId))
      .returning();

    if (!updated) return c.json({ error: "Menu item not found" }, 404);
    return c.json({ data: updated } as any, 200);
  } catch (error) {
    console.error("Failed to update menu item:", error);
    return c.json({ error: "Failed to update menu item" }, 500);
  }
});
