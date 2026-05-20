import { relations } from "drizzle-orm";
import {
  integer,
  pgTable,
  varchar,
  pgEnum,
  timestamp,
  boolean,
  index,
} from "drizzle-orm/pg-core";

export const timestamps = {
  createdAt: timestamp("created_at").notNull().defaultNow(),
  updatedAt: timestamp("updated_at").notNull().defaultNow(),
};

export const userRoleEnum = pgEnum("role", [
  "CUSTOMER",
  "COURIER",
  "RESTAURANT_OWNER",
  "ADMIN",
]);

export const restaurantStatusEnum = pgEnum("restaurant_status", ["OPEN", "CLOSED"]);

export const orderStatusEnum = pgEnum("order_status", [
  "PENDING",
  "PREPARING",
  "READY_FOR_DELIVERY",
  "DELIVERY_IN_PROGRESS",
  "DELIVERED",
  "CANCELLED",
]);

export const deliveryStatusEnum = pgEnum("delivery_status", [
  "ASSIGNED",
  "PICKED_UP",
  "DELIVERED",
  "CANCELLED",
]);

// identity domain

export const usersTable = pgTable(
  "users",
  {
    id: integer().primaryKey().generatedAlwaysAsIdentity(),
    email: varchar({ length: 255 }).notNull().unique(),
    password: varchar({ length: 255 }).notNull(),
    name: varchar({ length: 255 }).notNull(),
    age: integer().notNull(),
    role: userRoleEnum().notNull().default("CUSTOMER"),
    ...timestamps,
  },
  (table) => ({
    emailIndex: index("users_email_idx").on(table.email),
  }),
);

// multi-tenancy domain

export const restaurantsTable = pgTable(
  "restaurants",
  {
    id: integer().primaryKey().generatedAlwaysAsIdentity(),
    owner_id: integer()
      .notNull()
      .references(() => usersTable.id),
    name: varchar({ length: 255 }).notNull(),
    address: varchar({ length: 255 }).notNull(),
    status: restaurantStatusEnum().notNull().default("OPEN"),
    image_url: varchar({ length: 255 }).notNull(),
    ...timestamps,
  },
  (table) => ({
    ownerIndex: index("restaurants_owner_id_idx").on(table.owner_id),
  }),
);

// catalog domain

export const categoriesTable = pgTable(
  "categories",
  {
    id: integer().primaryKey().generatedAlwaysAsIdentity(),
    restaurant_id: integer()
      .notNull()
      .references(() => restaurantsTable.id),
    name: varchar({ length: 255 }).notNull(),
    sort_order: integer().notNull().default(0),
    ...timestamps,
  },
  (table) => ({
    restaurantIndex: index("categories_restaurant_id_idx").on(
      table.restaurant_id,
    ),
  }),
);

export const menuItemsTable = pgTable(
  "menu_items",
  {
    id: integer().primaryKey().generatedAlwaysAsIdentity(),
    restaurant_id: integer()
      .notNull()
      .references(() => restaurantsTable.id),
    category_id: integer()
      .notNull()
      .references(() => categoriesTable.id),
    name: varchar({ length: 255 }).notNull(),
    description: varchar({ length: 255 }).notNull(),
    is_available: boolean().notNull().default(true),
    price: integer().notNull(),
    image_url: varchar({ length: 255 }).notNull(),
    ...timestamps,
  },
  (table) => ({
    restaurantIndex: index("menu_items_restaurant_id_idx").on(
      table.restaurant_id,
    ),
    categoryIndex: index("menu_items_category_id_idx").on(table.category_id),
  }),
);

// transaction domain

export const ordersTable = pgTable(
  "orders",
  {
    id: integer().primaryKey().generatedAlwaysAsIdentity(),
    user_id: integer()
      .notNull()
      .references(() => usersTable.id),
    restaurant_id: integer()
      .notNull()
      .references(() => restaurantsTable.id),
    status: orderStatusEnum().notNull().default("PENDING"),
    total_amount: integer().notNull(),
    delivery_address: varchar({ length: 255 }).notNull(),
    ...timestamps,
  },
  (table) => ({
    userIndex: index("orders_user_id_idx").on(table.user_id),
    restaurantIndex: index("orders_restaurant_id_idx").on(table.restaurant_id),
  }),
);

export const orderItemsTable = pgTable(
  "order_items",
  {
    id: integer().primaryKey().generatedAlwaysAsIdentity(),
    order_id: integer()
      .notNull()
      .references(() => ordersTable.id),
    menu_item_id: integer()
      .notNull()
      .references(() => menuItemsTable.id),
    quantity: integer().notNull(),
    unit_price_at_purchase: integer().notNull(),
    ...timestamps,
  },
  (table) => ({
    orderIndex: index("order_items_order_id_idx").on(table.order_id),
  }),
);

// delivery domain
export const deliveriesTable = pgTable(
  "deliveries",
  {
    id: integer().primaryKey().generatedAlwaysAsIdentity(),
    order_id: integer()
      .notNull()
      .references(() => ordersTable.id),
    courier_id: integer()
      .notNull()
      .references(() => usersTable.id),
    status: deliveryStatusEnum().notNull().default("ASSIGNED"),
    picked_up_at: timestamp({ mode: "date" }),
    delivered_at: timestamp({ mode: "date" }),
    ...timestamps,
  },
  (table) => ({
    orderIndex: index("deliveries_order_id_idx").on(table.order_id),
    courierIndex: index("deliveries_courier_id_idx").on(table.courier_id),
  }),
);

// relations
export const userRelations = relations(usersTable, ({ many }) => ({
  restaurants: many(restaurantsTable),
  orders: many(ordersTable),
  deliveries: many(deliveriesTable),
}));

export const restaurantRelations = relations(
  restaurantsTable,
  ({ one, many }) => ({
    owner: one(usersTable, {
      fields: [restaurantsTable.owner_id],
      references: [usersTable.id],
    }),
    categories: many(categoriesTable),
    menuItems: many(menuItemsTable),
    orders: many(ordersTable),
  }),
);

export const categoryRelations = relations(
  categoriesTable,
  ({ one, many }) => ({
    restaurant: one(restaurantsTable, {
      fields: [categoriesTable.restaurant_id],
      references: [restaurantsTable.id],
    }),
    menuItems: many(menuItemsTable),
  }),
);
// TODO
export const menuItemRelations = relations(menuItemsTable, ({ many }) => ({
  orderItems: many(orderItemsTable),
}));

export const orderRelations = relations(ordersTable, ({ many }) => ({
  orderItems: many(orderItemsTable),
  deliveries: many(deliveriesTable),
}));

export const deliveryRelations = relations(deliveriesTable, ({ many }) => ({
  orders: many(ordersTable),
}));
