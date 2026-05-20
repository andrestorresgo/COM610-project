import { OpenAPIHono, createRoute, z } from "@hono/zod-openapi";
import { eq } from "drizzle-orm";
import { db } from "../db/index";
import { categoriesTable } from "../db/schema";
import { createCategoryDto, updateCategoryDto } from "../dto/category.dto";
import { customValidationHook } from "../lib/hooks";

export const categoriesRouter = new OpenAPIHono({
  defaultHook: customValidationHook,
});

// ── Reusable schemas ────────────────────────────────────────────────

const ErrorSchema = z
  .object({ error: z.string() })
  .openapi("CategoryErrorResponse");

const CategorySchema = z
  .object({
    id: z.number(),
    restaurant_id: z.number(),
    name: z.string(),
    sort_order: z.number(),
    createdAt: z.string(),
    updatedAt: z.string(),
  })
  .openapi("Category");

// ── GET /restaurants/:restaurantId/categories ───────────────────────

const listByRestaurantRoute = createRoute({
  method: "get",
  path: "/restaurants/{restaurantId}/categories",
  tags: ["Categories"],
  summary: "List all categories for a restaurant",
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
          schema: z.object({ data: z.array(CategorySchema) }),
        },
      },
      description: "Categories for restaurant",
    },
    400: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid restaurant ID",
    },
  },
});

categoriesRouter.openapi(listByRestaurantRoute, async (c) => {
  const { restaurantId } = c.req.valid("param");
  const rId = parseInt(restaurantId, 10);
  if (isNaN(rId) || rId <= 0) {
    return c.json({ error: "Invalid restaurant ID" }, 400);
  }

  const all = await db
    .select()
    .from(categoriesTable)
    .where(eq(categoriesTable.restaurant_id, rId));

  return c.json({ data: all } as any, 200);
});

// ── POST /categories ────────────────────────────────────────────────

const createCategoryRoute = createRoute({
  method: "post",
  path: "/",
  tags: ["Categories"],
  summary: "Create a category",
  request: {
    body: {
      content: { "application/json": { schema: createCategoryDto } },
      required: true,
      description: "Category payload",
    },
  },
  responses: {
    201: {
      content: {
        "application/json": {
          schema: z.object({ data: CategorySchema }),
        },
      },
      description: "Category created",
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

categoriesRouter.openapi(createCategoryRoute, async (c) => {
  const body = c.req.valid("json");
  try {
    const [result] = await db.insert(categoriesTable).values(body).returning();
    return c.json({ data: result } as any, 201);
  } catch (error) {
    console.error("Failed to create category:", error);
    return c.json({ error: "Failed to create category" }, 500);
  }
});

// ── PATCH /categories/:id ───────────────────────────────────────────

const updateRoute = createRoute({
  method: "patch",
  path: "/{id}",
  tags: ["Categories"],
  summary: "Update a category",
  request: {
    params: z.object({
      id: z
        .string()
        .regex(/^\d+$/, "ID must be a numeric string")
        .openapi({ param: { name: "id", in: "path" }, example: "1" }),
    }),
    body: {
      content: { "application/json": { schema: updateCategoryDto } },
      required: true,
      description: "Fields to update",
    },
  },
  responses: {
    200: {
      content: {
        "application/json": {
          schema: z.object({ data: CategorySchema }),
        },
      },
      description: "Category updated",
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

categoriesRouter.openapi(updateRoute, async (c) => {
  const { id } = c.req.valid("param");
  const numId = parseInt(id, 10);
  if (isNaN(numId) || numId <= 0) {
    return c.json({ error: "Invalid category ID" }, 400);
  }

  const body = c.req.valid("json");
  try {
    const [updated] = await db
      .update(categoriesTable)
      .set({ ...body, updatedAt: new Date() })
      .where(eq(categoriesTable.id, numId))
      .returning();

    if (!updated) return c.json({ error: "Category not found" }, 404);
    return c.json({ data: updated } as any, 200);
  } catch (error) {
    console.error("Failed to update category:", error);
    return c.json({ error: "Failed to update category" }, 500);
  }
});

// ── DELETE /categories/:id ──────────────────────────────────────────

const deleteRoute = createRoute({
  method: "delete",
  path: "/{id}",
  tags: ["Categories"],
  summary: "Delete a category",
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
          schema: z.object({ data: z.object({ success: z.boolean() }) }),
        },
      },
      description: "Category deleted",
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
      description: "Deletion failed",
    },
  },
});

categoriesRouter.openapi(deleteRoute, async (c) => {
  const { id } = c.req.valid("param");
  const numId = parseInt(id, 10);
  if (isNaN(numId) || numId <= 0) {
    return c.json({ error: "Invalid category ID" }, 400);
  }

  try {
    const [deleted] = await db
      .delete(categoriesTable)
      .where(eq(categoriesTable.id, numId))
      .returning();

    if (!deleted) return c.json({ error: "Category not found" }, 404);
    return c.json({ data: { success: true } } as any, 200);
  } catch (error) {
    console.error("Failed to delete category:", error);
    return c.json({ error: "Failed to delete category" }, 500);
  }
});
