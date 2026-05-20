import { z } from "@hono/zod-openapi";

export const createMenuItemDto = z.object({
  restaurant_id: z.number().int().positive(),
  category_id: z.number().int().positive(),
  name: z.string().min(1).max(255),
  description: z.string().min(1).max(255),
  is_available: z.boolean().optional(),
  price: z.number().int().nonnegative(),
  image_url: z.string().url().max(255),
});

export const updateMenuItemDto = z.object({
  name: z.string().min(1).max(255).optional(),
  description: z.string().min(1).max(255).optional(),
  is_available: z.boolean().optional(),
  price: z.number().int().nonnegative().optional(),
  image_url: z.string().url().max(255).optional(),
});
