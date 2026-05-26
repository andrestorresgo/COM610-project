import { OpenAPIHono, createRoute, z } from "@hono/zod-openapi";
import { PutObjectCommand } from "@aws-sdk/client-s3";
import { getSignedUrl } from "@aws-sdk/s3-request-presigner";
import { eq } from "drizzle-orm";
import { s3Client } from "../services/aws";
import { db } from "../db/index";
import { restaurantsTable, menuItemsTable } from "../db/schema";
import { customValidationHook } from "../lib/hooks";

export const imagesRouter = new OpenAPIHono({
  defaultHook: customValidationHook,
});

// ── Whitelist & Input Validation ────────────────────────────────────
const ALLOWED_MIME_TYPES = [
  "image/jpeg",
  "image/jpg",
  "image/png",
  "image/gif",
  "image/webp",
];

const ALLOWED_ENTITY_TYPES = ["restaurant", "menu-item"] as const;

// ── Schemas ─────────────────────────────────────────────────────────

const UploadQuerySchema = z.object({
  contentType: z
    .string()
    .openapi({
      param: { name: "contentType", in: "query" },
      example: "image/png",
      description: "The MIME type of the file (e.g. image/jpeg, image/png)",
    }),
  entityType: z
    .enum(ALLOWED_ENTITY_TYPES)
    .openapi({
      param: { name: "entityType", in: "query" },
      example: "restaurant",
      description: "The type of entity this image belongs to",
    }),
  entityId: z
    .string()
    .regex(/^\d+$/, "Must be a numeric ID")
    .openapi({
      param: { name: "entityId", in: "query" },
      example: "1",
      description: "The numeric ID of the entity this image belongs to",
    }),
});

const ErrorSchema = z
  .object({
    error: z.string(),
  })
  .openapi("ImageErrorResponse");

const UploadUrlResponseSchema = z
  .object({
    uploadUrl: z.string().openapi({ example: "https://agachadeats-raw-assets-7552121.s3.us-east-2.amazonaws.com/uploads/..." }),
    fileKey: z.string().openapi({ example: "uploads/a0b1c2-d3e4-f5g6.png" }),
  })
  .openapi("UploadUrlResponse");

const WebhookBodySchema = z
  .object({
    sourceKey: z.string(),
    optimizedKey: z.string(),
    optimizedUrl: z.string(),
    metadata: z.record(z.string()).optional(),
  })
  .openapi("WebhookPayload");

const WebhookResponseSchema = z
  .object({
    success: z.boolean(),
    entityType: z.string().optional(),
    entityId: z.number().optional(),
  })
  .openapi("WebhookResponse");

// ── GET /images/upload-url ──────────────────────────────────────────

const getUploadUrlRoute = createRoute({
  method: "get",
  path: "/upload-url",
  tags: ["Images"],
  summary: "Generate an S3 Pre-signed URL for direct upload",
  description:
    "Request a pre-signed URL to upload an image directly to S3. " +
    "The entity metadata (entityType + entityId) is embedded as S3 object metadata " +
    "so the Lambda optimizer can route the webhook callback. URL expires in 60 seconds.",
  request: {
    query: UploadQuerySchema,
  },
  responses: {
    200: {
      content: {
        "application/json": {
          schema: UploadUrlResponseSchema,
        },
      },
      description: "Successfully generated upload URL",
    },
    400: {
      content: {
        "application/json": {
          schema: ErrorSchema,
        },
      },
      description: "Invalid content type or query parameters",
    },
    500: {
      content: {
        "application/json": {
          schema: ErrorSchema,
        },
      },
      description: "Internal server error generating URL",
    },
  },
});

imagesRouter.openapi(getUploadUrlRoute, async (c) => {
  const { contentType, entityType, entityId } = c.req.valid("query");

  const normalizedContentType = contentType.toLowerCase().trim();

  // Strict MIME type validation
  if (!ALLOWED_MIME_TYPES.includes(normalizedContentType)) {
    return c.json(
      {
        error: `Unsupported Content-Type '${contentType}'. Whitelisted: ${ALLOWED_MIME_TYPES.join(", ")}`,
      },
      400
    );
  }

  // Determine file extension
  let extension = "jpg";
  if (normalizedContentType.includes("png")) {
    extension = "png";
  } else if (normalizedContentType.includes("gif")) {
    extension = "gif";
  } else if (normalizedContentType.includes("webp")) {
    extension = "webp";
  } else if (normalizedContentType.includes("jpeg") || normalizedContentType.includes("jpg")) {
    extension = "jpg";
  }

  const uniqueFileName = `${crypto.randomUUID()}.${extension}`;
  const fileKey = `uploads/${uniqueFileName}`;
  const bucketName = process.env.AWS_S3_BUCKET_NAME || "agachadeats-raw-assets-7552121";

  try {
    const command = new PutObjectCommand({
      Bucket: bucketName,
      Key: fileKey,
      ContentType: normalizedContentType,
      // Embed entity metadata so the Lambda can forward it in the webhook callback.
      // S3 automatically prefixes these with "x-amz-meta-" in the HTTP layer,
      // but the SDK expects plain keys here.
      Metadata: {
        "entity-type": entityType,
        "entity-id": entityId,
      },
    });

    // Generate URL with strict 60 seconds expiration
    const uploadUrl = await getSignedUrl(s3Client, command, { expiresIn: 60 });

    return c.json(
      {
        uploadUrl,
        fileKey,
      },
      200
    );
  } catch (error: any) {
    console.error("Failed to generate presigned S3 URL:", error);
    return c.json(
      {
        error: "Failed to generate upload URL: " + (error.message || String(error)),
      },
      500
    );
  }
});

// ── POST /images/webhook ────────────────────────────────────────────

const webhookRoute = createRoute({
  method: "post",
  path: "/webhook",
  tags: ["Images"],
  summary: "Lambda webhook callback — updates entity image URL",
  description:
    "Called by the Lambda image optimizer after compressing an image. " +
    "Verifies the HMAC-SHA256 signature (if WEBHOOK_SECRET is configured), " +
    "then updates the matching restaurant or menu-item record with the optimized URL.",
  request: {
    body: {
      content: { "application/json": { schema: WebhookBodySchema } },
      required: true,
      description: "Webhook payload from Lambda",
    },
  },
  responses: {
    200: {
      content: {
        "application/json": { schema: WebhookResponseSchema },
      },
      description: "Image URL updated successfully",
    },
    400: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid payload or missing metadata",
    },
    401: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid webhook signature",
    },
    404: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Entity not found",
    },
    500: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Internal server error",
    },
  },
});

imagesRouter.openapi(webhookRoute, async (c) => {
  // ── 1. Verify HMAC signature (if secret is configured) ──────────
  const webhookSecret = process.env.WEBHOOK_SECRET;
  if (webhookSecret) {
    const signatureHeader = c.req.header("x-webhook-signature");
    if (!signatureHeader) {
      return c.json({ error: "Missing X-Webhook-Signature header" }, 401);
    }

    // Read raw body for HMAC verification
    const rawBody = await c.req.raw.clone().text();
    const key = await crypto.subtle.importKey(
      "raw",
      new TextEncoder().encode(webhookSecret),
      { name: "HMAC", hash: "SHA-256" },
      false,
      ["sign"]
    );
    const sig = await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(rawBody));
    const expectedSignature = Array.from(new Uint8Array(sig))
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("");

    if (signatureHeader !== expectedSignature) {
      console.warn("Webhook signature mismatch");
      return c.json({ error: "Invalid webhook signature" }, 401);
    }
  }

  // ── 2. Parse payload ────────────────────────────────────────────
  const body = c.req.valid("json");
  const metadata = body.metadata || {};
  const entityType = metadata["entity-type"];
  const entityId = metadata["entity-id"];
  const optimizedUrl = body.optimizedUrl;

  if (!entityType || !entityId) {
    return c.json(
      { error: "Missing entity-type or entity-id in webhook metadata" },
      400
    );
  }

  const numericId = parseInt(entityId, 10);
  if (isNaN(numericId) || numericId <= 0) {
    return c.json({ error: "Invalid entity-id" }, 400);
  }

  // ── 3. Update the correct entity ───────────────────────────────
  try {
    if (entityType === "restaurant") {
      const [updated] = await db
        .update(restaurantsTable)
        .set({ image_url: optimizedUrl, updatedAt: new Date() })
        .where(eq(restaurantsTable.id, numericId))
        .returning();

      if (!updated) {
        return c.json({ error: `Restaurant ${numericId} not found` }, 404);
      }

      console.log(`✓ Updated restaurant ${numericId} image_url → ${optimizedUrl}`);
      return c.json({ success: true, entityType: "restaurant", entityId: numericId }, 200);

    } else if (entityType === "menu-item") {
      const [updated] = await db
        .update(menuItemsTable)
        .set({ image_url: optimizedUrl, updatedAt: new Date() })
        .where(eq(menuItemsTable.id, numericId))
        .returning();

      if (!updated) {
        return c.json({ error: `Menu item ${numericId} not found` }, 404);
      }

      console.log(`✓ Updated menu-item ${numericId} image_url → ${optimizedUrl}`);
      return c.json({ success: true, entityType: "menu-item", entityId: numericId }, 200);

    } else {
      return c.json({ error: `Unknown entity-type: ${entityType}` }, 400);
    }
  } catch (error: any) {
    console.error("Webhook DB update failed:", error);
    return c.json(
      { error: "Failed to update image URL: " + (error.message || String(error)) },
      500
    );
  }
});

// ── POST /images/upload ─────────────────────────────────────────────
// Server-side proxy upload for testing from Swagger UI.
// Accepts a file via multipart/form-data, uploads raw binary to S3,
// which then triggers the Lambda optimization pipeline.

const DirectUploadResponseSchema = z
  .object({
    fileKey: z.string().openapi({ example: "uploads/a0b1c2-d3e4-f5g6.png" }),
    bucket: z.string().openapi({ example: "agachadeats-raw-assets-7552121" }),
    message: z.string().openapi({ example: "File uploaded to S3. Lambda will process it shortly." }),
  })
  .openapi("DirectUploadResponse");

const directUploadRoute = createRoute({
  method: "post",
  path: "/upload",
  tags: ["Images"],
  summary: "Upload an image file directly, only to test Lambda",
  description:
    "Server-side proxy upload — accepts a file via multipart form, " +
    "uploads the raw binary to S3 with entity metadata, and returns the file key. " +
    "The S3 upload triggers the Lambda optimizer automatically.",
  request: {
    query: z.object({
      entityType: z
        .enum(ALLOWED_ENTITY_TYPES)
        .openapi({
          param: { name: "entityType", in: "query" },
          example: "restaurant",
          description: "The type of entity this image belongs to",
        }),
      entityId: z
        .string()
        .regex(/^\d+$/, "Must be a numeric ID")
        .openapi({
          param: { name: "entityId", in: "query" },
          example: "1",
          description: "The numeric ID of the entity this image belongs to",
        }),
    }),
    body: {
      content: {
        "multipart/form-data": {
          schema: z.object({
            file: z
              .any()
              .openapi({ type: "string", format: "binary", description: "The image file to upload" }),
          }),
        },
      },
      required: true,
      description: "Image file as multipart/form-data",
    },
  },
  responses: {
    200: {
      content: {
        "application/json": { schema: DirectUploadResponseSchema },
      },
      description: "File uploaded to S3 successfully",
    },
    400: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "Invalid file or parameters",
    },
    500: {
      content: { "application/json": { schema: ErrorSchema } },
      description: "S3 upload failed",
    },
  },
});

imagesRouter.openapi(directUploadRoute, async (c) => {
  const { entityType, entityId } = c.req.valid("query");

  // Parse the multipart body to get the file
  const body = await c.req.parseBody();
  const file = body["file"];

  if (!file || !(file instanceof File)) {
    return c.json({ error: "No file provided. Upload a file in the 'file' field." }, 400);
  }

  // Validate MIME type
  const contentType = file.type.toLowerCase();
  if (!ALLOWED_MIME_TYPES.includes(contentType)) {
    return c.json(
      {
        error: `Unsupported file type '${file.type}'. Allowed: ${ALLOWED_MIME_TYPES.join(", ")}`,
      },
      400
    );
  }

  // Determine extension from MIME type
  let extension = "jpg";
  if (contentType.includes("png")) extension = "png";
  else if (contentType.includes("gif")) extension = "gif";
  else if (contentType.includes("webp")) extension = "webp";

  const uniqueFileName = `${crypto.randomUUID()}.${extension}`;
  const fileKey = `uploads/${uniqueFileName}`;
  const bucketName = process.env.AWS_S3_BUCKET_NAME || "agachadeats-raw-assets-7552121";

  try {
    // Read the file as raw binary
    const fileBuffer = await file.arrayBuffer();

    const command = new PutObjectCommand({
      Bucket: bucketName,
      Key: fileKey,
      Body: new Uint8Array(fileBuffer),
      ContentType: contentType,
      Metadata: {
        "entity-type": entityType,
        "entity-id": entityId,
      },
    });

    await s3Client.send(command);

    console.log(`✓ Uploaded ${fileKey} (${file.size} bytes, ${contentType}) for ${entityType}:${entityId}`);

    return c.json(
      {
        fileKey,
        bucket: bucketName,
        message: "File uploaded to S3. Lambda will process it shortly.",
      },
      200
    );
  } catch (error: any) {
    console.error("Failed to upload file to S3:", error);
    return c.json(
      { error: "S3 upload failed: " + (error.message || String(error)) },
      500
    );
  }
});
