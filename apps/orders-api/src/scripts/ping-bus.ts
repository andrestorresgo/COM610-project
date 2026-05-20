import { publishEvent, closeRabbitMQ } from "../lib/rabbitmq";

async function ping() {
  try {
    const payload = { msg: "ping", timestamp: Date.now() };
    const routingKey = "system.ping";

    console.log(`Publishing ping to ${routingKey}...`);
    await publishEvent(routingKey, payload);
    console.log("Ping sent successfully!");
  } catch (error) {
    console.error("Failed to send ping:", error);
  } finally {
    await closeRabbitMQ();
  }
}

ping();
