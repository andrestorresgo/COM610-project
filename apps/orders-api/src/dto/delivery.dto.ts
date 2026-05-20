import { z } from "@hono/zod-openapi";
import { deliveryStatusEnum } from "../db/schema";

export const createDeliveryDto = z.object({
  order_id: z.number().int().positive(),
  courier_id: z.number().int().positive(),
  status: z.enum(deliveryStatusEnum.enumValues).optional(),
});

export const updateDeliveryStatusDto = z.object({
  status: z.enum(deliveryStatusEnum.enumValues),
});
