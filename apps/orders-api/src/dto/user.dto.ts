import { z } from "@hono/zod-openapi";
import { userRoleEnum } from "../db/schema";

export const createUserDto = z.object({
  email: z.string().email().min(1).max(255),
  password: z.string().min(6).max(255),
  name: z.string().min(1).max(255),
  age: z.number().int().nonnegative(),
  role: z.enum(userRoleEnum.enumValues).optional(),
});

export const updateUserDto = z.object({
  email: z.string().email().min(1).max(255).optional(),
  password: z.string().min(6).max(255).optional(),
  name: z.string().min(1).max(255).optional(),
  age: z.number().int().nonnegative().optional(),
  role: z.enum(userRoleEnum.enumValues).optional(),
});
