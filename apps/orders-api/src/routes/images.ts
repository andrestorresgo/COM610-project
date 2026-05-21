import { OpenAPIHono, createRoute, z } from "@hono/zod-openapi";
import { PutObjectCommand } from "@aws-sdk/client-s3";
import { getSignedUrl } from "@aws-sdk/s3-request-presigner";
import { s3Client } from "../services/aws";
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

const QuerySchema = z.object({
  contentType: z
    .string()
    .openapi({
      param: {
        name: "contentType",
        in: "query",
      },
      example: "image/png",
      description: "The MIME type of the file (e.g. image/jpeg, image/png)",
    }),
});

const ErrorSchema = z
  .object({
    error: z.string(),
  })
  .openapi("ImageErrorResponse");

const UploadUrlResponseSchema = z
  .object({
    uploadUrl: z.string().openapi({ example: "https://agachadeats-raw-assets-7552121.s3.us-east-2.amazonaws.com/uploads/... " }),
    fileKey: z.string().openapi({ example: "uploads/a0b1c2-d3e4-f5g6.png" }),
  })
  .openapi("UploadUrlResponse");

// ── GET /images/upload-url ──────────────────────────────────────────
const getUploadUrlRoute = createRoute({
  method: "get",
  path: "/upload-url",
  tags: ["Images"],
  summary: "Generate an S3 Pre-signed URL for direct upload",
  description: "Request a pre-signed URL to upload an image directly to S3. URL expires in 60 seconds.",
  request: {
    query: QuerySchema,
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
  const { contentType } = c.req.valid("query");

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
