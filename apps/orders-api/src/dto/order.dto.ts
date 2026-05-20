import { z } from "@hono/zod-openapi";
import { orderStatusEnum } from "../db/schema";

export const createOrderDto = z.object({
  user_id: z.number().int().positive(),
  restaurant_id: z.number().int().positive(),
  total_amount: z.number().int().nonnegative(),
  delivery_address: z.string().min(1),
  items: z.array(
    z.object({
      menu_item_id: z.number().int().positive(),
      quantity: z.number().int().positive(),
      unit_price_at_purchase: z.number().int().nonnegative(),
    })
  ).min(1),
});

export const updateOrderStatusDto = z.object({
  status: z.enum(orderStatusEnum.enumValues),
});
