import { Hook } from "@hono/zod-openapi";

export const customValidationHook: Hook<any, any, any, any> = (result, c) => {
  if (!result.success) {
    return c.json(
      {
        success: false,
        error: {
          name: "ValidationError",
          message: "Request validation failed",
          issues: result.error.issues,
        },
      },
      400,
    );
  }
};
