import * as amqp from "amqplib";

const RABBITMQ_URL =
  process.env.RABBITMQ_URL || "amqp://guest:guest@rabbitmq:5672/";
const EXCHANGE_NAME = "agachadeats_events";

type Connection = Awaited<ReturnType<typeof amqp.connect>>;
type Channel = Awaited<ReturnType<Connection["createChannel"]>>;

let connection: Connection | null = null;
let channel: Channel | null = null;
let connectingPromise: Promise<{ connection: Connection; channel: Channel }> | null = null;

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

/**
 * Connects to RabbitMQ with retry logic.
 * Returns a singleton connection and channel.
 */
export async function connectRabbitMQ(retries = 5): Promise<{ connection: Connection; channel: Channel }> {
  // If we already have a connection and channel, return them
  if (connection && channel) {
    return { connection, channel };
  }

  // If a connection attempt is already in progress, wait for it
  if (connectingPromise) {
    return connectingPromise;
  }

  connectingPromise = (async () => {
    for (let i = 0; i < retries; i++) {
      try {
        console.log(
          `Attempting to connect to RabbitMQ (Attempt ${i + 1}/${retries})...`,
        );
        
        // amqplib promise API
        const conn = await amqp.connect(RABBITMQ_URL);
        const ch = await conn.createChannel();

        // Define "Topic" exchange
        await ch.assertExchange(EXCHANGE_NAME, "topic", { durable: true });

        console.log("Successfully connected to RabbitMQ");

        // Handle connection events
        conn.on("error", (err: Error) => {
          console.error("RabbitMQ connection error:", err);
          connection = null;
          channel = null;
        });

        conn.on("close", () => {
          console.warn("RabbitMQ connection closed.");
          connection = null;
          channel = null;
          connectingPromise = null;
        });

        connection = conn;
        channel = ch;
        return { connection, channel };
      } catch (error) {
        console.error(`RabbitMQ connection failed: ${(error as Error).message}`);
        if (i < retries - 1) {
          const waitTime = Math.pow(2, i) * 1000;
          console.log(`Retrying in ${waitTime / 1000} seconds...`);
          await sleep(waitTime);
        } else {
          connectingPromise = null;
          throw new Error(`Failed to connect to RabbitMQ after ${retries} retries`);
        }
      }
    }
    connectingPromise = null;
    throw new Error("Failed to connect to RabbitMQ");
  })();

  return connectingPromise;
}

/**
 * Publishes an event to the RabbitMQ exchange.
 */
export async function publishEvent(routingKey: string, payload: any) {
  const { channel: ch } = await connectRabbitMQ();
  
  const message = Buffer.from(JSON.stringify(payload));
  const success = ch.publish(EXCHANGE_NAME, routingKey, message, {
    persistent: true,
  });

  if (!success) {
    console.warn("Failed to publish message immediately, buffer might be full");
  }
  return success;
}

/**
 * Closes the RabbitMQ connection and channel.
 */
export async function closeRabbitMQ() {
  if (channel) {
    try {
      await channel.close();
    } catch (err) {
      console.error("Error closing RabbitMQ channel:", err);
    }
    channel = null;
  }
  if (connection) {
    try {
      await connection.close();
    } catch (err) {
      console.error("Error closing RabbitMQ connection:", err);
    }
    connection = null;
  }
  connectingPromise = null;
}
