CREATE TYPE "public"."delivery_status" AS ENUM('ASSIGNED', 'PICKED_UP', 'DELIVERED', 'CANCELLED');--> statement-breakpoint
CREATE TYPE "public"."order_status" AS ENUM('PENDING', 'PREPARING', 'READY_FOR_DELIVERY', 'DELIVERY_IN_PROGRESS', 'DELIVERED', 'CANCELLED');--> statement-breakpoint
CREATE TYPE "public"."restaurant_status" AS ENUM('OPEN', 'CLOSED');--> statement-breakpoint
ALTER TABLE "deliveries" ALTER COLUMN "status" DROP DEFAULT;--> statement-breakpoint
ALTER TABLE "deliveries" ALTER COLUMN "status" SET DATA TYPE "public"."delivery_status" USING "status"::text::"public"."delivery_status";--> statement-breakpoint
ALTER TABLE "deliveries" ALTER COLUMN "status" SET DEFAULT 'ASSIGNED';--> statement-breakpoint
ALTER TABLE "orders" ALTER COLUMN "status" DROP DEFAULT;--> statement-breakpoint
ALTER TABLE "orders" ALTER COLUMN "status" SET DATA TYPE "public"."order_status" USING "status"::text::"public"."order_status";--> statement-breakpoint
ALTER TABLE "orders" ALTER COLUMN "status" SET DEFAULT 'PENDING';--> statement-breakpoint
ALTER TABLE "restaurants" ALTER COLUMN "status" DROP DEFAULT;--> statement-breakpoint
ALTER TABLE "restaurants" ALTER COLUMN "status" SET DATA TYPE "public"."restaurant_status" USING "status"::text::"public"."restaurant_status";--> statement-breakpoint
ALTER TABLE "restaurants" ALTER COLUMN "status" SET DEFAULT 'OPEN';--> statement-breakpoint
DROP TYPE "public"."status";