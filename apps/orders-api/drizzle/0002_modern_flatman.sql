ALTER TABLE "menu_items" ALTER COLUMN "image_url" SET DATA TYPE varchar(512);--> statement-breakpoint
ALTER TABLE "menu_items" ALTER COLUMN "image_url" SET DEFAULT '';--> statement-breakpoint
ALTER TABLE "restaurants" ALTER COLUMN "image_url" SET DATA TYPE varchar(512);--> statement-breakpoint
ALTER TABLE "restaurants" ALTER COLUMN "image_url" SET DEFAULT '';