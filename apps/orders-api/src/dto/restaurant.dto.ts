import { z } from "@hono/zod-openapi";
import { restaurantStatusEnum } from "../db/schema";

export const createRestaurantDto = z.object({
  owner_id: z.number().int().positive(),
  name: z.string().min(1).max(255),
  address: z.string().min(1).max(255),
  image_url: z.string().max(512).optional(),
  status: z.enum(restaurantStatusEnum.enumValues).optional(),
});

export const updateRestaurantDto = z.object({
  name: z.string().min(1).max(255).optional(),
  address: z.string().min(1).max(255).optional(),
  image_url: z.string().max(512).optional(),
  status: z.enum(restaurantStatusEnum.enumValues).optional(),
});
