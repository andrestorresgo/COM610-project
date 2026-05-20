import { OpenAPIHono, createRoute, z } from "@hono/zod-openapi";
import { eq } from "drizzle-orm";
import { db } from "../db/index";
import { usersTable } from "../db/schema";
import { createUserDto, updateUserDto } from "../dto/user.dto";
import { customValidationHook } from "../lib/hooks";

export const usersRouter = new OpenAPIHono({
  defaultHook: customValidationHook,
});

// ── Reusable schemas ────────────────────────────────────────────────

const ErrorSchema = z
  .object({ error: z.string() })
  .openapi("UserErrorResponse");

const UserSchema = z
  .object({
    id: z.number(),
    email: z.string(),
    name: z.string(),
    age: z.number(),
    role: z.string(),
    createdAt: z.string(),
    updatedAt: z.string(),
  })
  .openapi("User");

// ── GET /users ──────────────────────────────────────────────────────

const listRoute = createRoute({
  method: "get",
  path: "/",
  tags: ["Users"],
  summary: "List all users",
  responses: {
    200: {
      content: {
        "application/json": {
          schema: z.object({ data: z.array(UserSchema) }),
        },
      },
      description: "All users (password omitted)",
    },
  },
});

usersRouter.openapi(listRoute, async (c) => {
  const allUsers = await db
    .select({
      id: usersTable.id,
      email: usersTable.email,
      name: usersTable.name,
      age: usersTable.age,
      role: usersTable.role,
      createdAt: usersTable.createdAt,
      updatedAt: usersTable.updatedAt,
    })
    .from(usersTable);
  return c.json({ data: allUsers } as any, 200);
});

// ── GET /users/:id ──────────────────────────────────────────────────

const getByIdRoute = createRoute({
  method: "get",
  path: "/{id}",
  tags: ["Users"],
  summary: "Get a user by ID",
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
          schema: z.object({ data: UserSchema }),
        },
      },
      description: "User found",
    },
    400: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid ID",
    },
    404: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "User not found",
    },
  },
});

usersRouter.openapi(getByIdRoute, async (c) => {
  const { id } = c.req.valid("param");
  const numId = parseInt(id, 10);
  if (isNaN(numId) || numId <= 0) {
    return c.json({ error: "Invalid user ID" }, 400);
  }

  const [user] = await db
    .select({
      id: usersTable.id,
      email: usersTable.email,
      name: usersTable.name,
      age: usersTable.age,
      role: usersTable.role,
      createdAt: usersTable.createdAt,
      updatedAt: usersTable.updatedAt,
    })
    .from(usersTable)
    .where(eq(usersTable.id, numId));

  if (!user) return c.json({ error: "User not found" }, 404);
  return c.json({ data: user } as any, 200);
});

// ── POST /users ─────────────────────────────────────────────────────

const createUserRoute = createRoute({
  method: "post",
  path: "/",
  tags: ["Users"],
  summary: "Create a user",
  request: {
    body: {
      content: { "application/json": { schema: createUserDto } },
      required: true,
      description: "User payload",
    },
  },
  responses: {
    201: {
      content: {
        "application/json": {
          schema: z.object({ data: UserSchema }),
        },
      },
      description: "User created",
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

usersRouter.openapi(createUserRoute, async (c) => {
  const body = c.req.valid("json");
  try {
    const [result] = await db.insert(usersTable).values(body).returning({
      id: usersTable.id,
      email: usersTable.email,
      name: usersTable.name,
      age: usersTable.age,
      role: usersTable.role,
      createdAt: usersTable.createdAt,
      updatedAt: usersTable.updatedAt,
    });
    return c.json({ data: result } as any, 201);
  } catch (error) {
    console.error("Failed to create user:", error);
    return c.json({ error: "Failed to create user" }, 500);
  }
});

// ── PATCH /users/:id ────────────────────────────────────────────────

const updateRoute = createRoute({
  method: "patch",
  path: "/{id}",
  tags: ["Users"],
  summary: "Update a user",
  request: {
    params: z.object({
      id: z
        .string()
        .regex(/^\d+$/, "ID must be a numeric string")
        .openapi({ param: { name: "id", in: "path" }, example: "1" }),
    }),
    body: {
      content: { "application/json": { schema: updateUserDto } },
      required: true,
      description: "Fields to update",
    },
  },
  responses: {
    200: {
      content: {
        "application/json": {
          schema: z.object({ data: UserSchema }),
        },
      },
      description: "User updated",
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

usersRouter.openapi(updateRoute, async (c) => {
  const { id } = c.req.valid("param");
  const numId = parseInt(id, 10);
  if (isNaN(numId) || numId <= 0) {
    return c.json({ error: "Invalid user ID" }, 400);
  }

  const body = c.req.valid("json");
  try {
    const [updated] = await db
      .update(usersTable)
      .set({ ...body, updatedAt: new Date() })
      .where(eq(usersTable.id, numId))
      .returning({
        id: usersTable.id,
        email: usersTable.email,
        name: usersTable.name,
        age: usersTable.age,
        role: usersTable.role,
        createdAt: usersTable.createdAt,
        updatedAt: usersTable.updatedAt,
      });

    if (!updated) return c.json({ error: "User not found" }, 404);
    return c.json({ data: updated } as any, 200);
  } catch (error) {
    console.error("Failed to update user:", error);
    return c.json({ error: "Failed to update user" }, 500);
  }
});
