import { z } from "@hono/zod-openapi";

export const createCategoryDto = z.object({
  restaurant_id: z.number().int().positive(),
  name: z.string().min(1).max(255),
  sort_order: z.number().int().optional(),
});

export const updateCategoryDto = z.object({
  name: z.string().min(1).max(255).optional(),
  sort_order: z.number().int().optional(),
});
