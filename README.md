# Influenzer Backend

Backend service for the **Influenzer** B2B Creator Marketplace Application. Built with Go (Golang) and Clean Architecture.

## Features
- **Authentication**: Email/Password & Social Login (Google).
- **Brand Workflow**: Campaign Management, Creator Search.
- **Creator Workflow**: Job Feed, Proposals, S3 Video Upload.
- **Chat**: Real-time messaging (HTTP + WebSocket).
- **Payments**: Escrow system with Razorpay (Orders, Webhooks, Payouts).
- **Wallet**: Transaction history.

## Tech Stack
- **Language**: Go 1.21+
- **Framework**: Gin Gonic
- **Database**: PostgreSQL (Neon DB)
- **ORM**: GORM
- **Payments**: Razorpay
- **Docs**: Swagger

## Setup

1. **Clone**:
   ```bash
   git clone https://github.com/knight099/influenzer.git
   cd influenzer-backend
   ```

2. **Env**:
   Copy `.env.example` to `.env` and fill in secrets.

3. **Run**:
   ```bash
   go run cmd/api/main.go
   ```
   Or via Docker:
   ```bash
   docker-compose up --build
   ```

4. **API Docs**:
   Visit `http://localhost:8080/swagger/index.html` after starting the server.
