**Título:** Implementación de una Arquitectura Híbrida y Orientada a Eventos para un Sistema de Reparto de Alta Concurrencia.

**Objetivo General:** Diseñar y desplegar una infraestructura distribuida mediante una estrategia de nube híbrida (On-Premise y Nube Pública), garantizando el procesamiento asíncrono, la escalabilidad y la interoperabilidad de microservicios para soportar aplicaciones en tiempo real.

**Descripción del Proyecto:** El proyecto consiste en el desarrollo y orquestación del backend para una plataforma de entregas (delivery) capaz de manejar picos de tráfico y sincronización GPS en tiempo real. La infraestructura base residirá en un clúster autoalojado que segmentará los dominios del sistema en contenedores independientes (una API principal y un servicio de rastreo por WebSockets), comunicados internamente a través de un bus de mensajes. La complejidad principal radicará en la creación de un "puente" hacia la nube pública, delegando cargas de trabajo pesadas (como el almacenamiento y procesamiento de imágenes) a funciones Serverless y buckets de objetos, aliviando así el consumo de recursos del servidor local.

**Tecnología a emplear:** Docker, Docker Compose, Hono, Go, RabbitMQ, PostgreSQL, Redis, y servicios de AWS (S3 y Lambda).

**Miembros del grupo:**

Torres González Andrés Alejandro

[Informe Final](./Final/InformeFinal.md)
