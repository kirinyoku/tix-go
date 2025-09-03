# TixGo: Event Ticketing Service (Educational Project)

**TixGo**, is an event ticket booking service developed as an educational endeavor. It's designed to consolidate knowledge in building a robust backend application using modern Go technologies and best practices.

## Overview

TixGo provides a RESTful API for managing events, venues, seats, and orders. It includes features such as seat reservations (holds), order confirmation, caching for performance, and rate limiting to prevent abuse.

## Technologies Used

*   **Go:** The primary programming language.
*   **Gin Web Framework:** For building the HTTP API.
*   **PostgreSQL (pgx):** The relational database for persistent storage, utilizing the `pgx` driver and toolkit.
*   **Redis (go-redis):** Used for caching, rate limiting, and idempotency management.
*   **Docker & Docker Compose:** For containerization and orchestration of the application, database, and Redis.
*   **Goose:** A database migration tool for managing schema changes.
*   **Swaggo:** For automatic generation of Swagger API documentation.

## Functionality (MVP)

*   Creating an event with a venue and seats (rows/seats).
*   Viewing events and available seats.
*   Creating a "hold" (temporary reservation for N minutes).
*   Confirming an order (transferring held seats to sold).
*   Canceling/postponing a hold.
*   Caching of event details, seat maps, and availability counters.
*   Rate limiting on creating holds/orders via Redis.

## API Endpoints

**Public API:**

*   `GET /events/:id`: Get event details.
*   `GET /events/:id/availability`: Get availability counters for an event.
*   `GET /events/:id/seats`: List seats for an event.
*   `POST /events/:id/holds`: Create a hold (reservation) for seats (idempotent).
*   `POST /orders/confirm`: Confirm an order.
*   `GET /orders/:id`: Get order details with tickets.

**Admin API (TODO: add admin middleware):**

*   `POST /admin/venues`: Create a new venue.
*   `POST /admin/venues/:id/seats`: Batch create seats for a venue.
*   `POST /admin/events`: Create a new event and initialize its seats.

**Health Check & Documentation:**

*   `GET /healthz`: Application health check.
*   `GET /swagger/*any`: Swagger UI for API documentation.