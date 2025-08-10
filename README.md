# Gundam Platform API

🎌 API backend cho nền tảng thương mại điện tử Gundam - nơi trao đổi, mua bán và đấu giá các mô hình Gundam.

## Tổng quan

Gundam Platform API là một hệ thống backend được xây dựng bằng Go, cung cấp các tính năng toàn diện cho việc:

- 🛒 **Thương mại điện tử**: Mua bán mô hình Gundam
- 🔄 **Trao đổi**: Trao đổi trực tiếp giữa các người dùng
- 🏺 **Đấu giá**: Đấu giá các mô hình Gundam hiếm
- 💳 **Thanh toán**: Tích hợp ví điện tử và ZaloPay
- 📦 **Vận chuyển**: Tích hợp với Giao Hàng Nhanh (GHN)

## Công nghệ sử dụng

### Backend Framework
- **Go 1.24**: Ngôn ngữ lập trình chính
- **Gin Framework**: HTTP web framework
- **PostgreSQL**: Cơ sở dữ liệu chính
- **Redis**: Cache và session storage
- **Firebase**: Push notifications

### Database & ORM
- **SQLC**: Type-safe SQL code generation
- **PostgreSQL Migration**: Database schema management
- **pgx/v5**: PostgreSQL driver cho Go

### External Services
- **Cloudinary**: Image/file storage
- **ZaloPay**: Payment gateway
- **Giao Hàng Nhanh (GHN)**: Shipping provider
- **Gmail SMTP**: Email service
- **Discord**: Notifications
- **Ngrok**: Webhook tunneling

### Background Processing
- **Asynq**: Background job processing với Redis
- **gocron**: Scheduled tasks
- **Server-Sent Events (SSE)**: Real-time communications

## Cấu trúc dự án

```
gundam-BE/
├── api/                    # HTTP handlers và routing
│   ├── server.go          # Server setup và middleware
│   ├── types.go           # Request/Response types
│   └── *.go               # API handlers cho từng module
├── internal/
│   ├── db/
│   │   ├── migrations/    # Database migrations
│   │   ├── queries/       # SQL queries
│   │   └── sqlc/          # Generated SQLC code
│   ├── delivery/          # GHN shipping integration
│   ├── event/             # SSE event system
│   ├── mailer/            # Email services
│   ├── notification/      # Push notification services
│   ├── order_tracking/    # Order status tracking
│   ├── storage/           # File storage (Cloudinary)
│   ├── token/             # JWT token management
│   ├── util/              # Utilities và configuration
│   ├── worker/            # Background job processing
│   └── zalopay/           # ZaloPay integration
├── docs/                  # Swagger API documentation
├── main.go               # Application entry point
├── Dockerfile            # Docker containerization
├── compose.yaml          # Docker Compose setup
└── Makefile              # Build commands
```

## Tính năng chính

### 🔐 Xác thực & Phân quyền
- JWT-based authentication
- Google OAuth integration
- Role-based access control (Member, Seller, Moderator, Admin)
- OTP verification (SMS & Email)

### 👥 Quản lý người dùng
- Đăng ký/đăng nhập người dùng
- Profile management với avatar upload
- Địa chỉ giao hàng multiple
- Hệ thống ví điện tử

### 🛍️ Thương mại điện tử
- Catalog sản phẩm Gundam với search/filter
- Giỏ hàng và checkout
- Quản lý đơn hàng với tracking
- Hệ thống đánh giá và feedback

### 🔄 Trao đổi (Exchange)
- Tạo bài đăng trao đổi
- Đề xuất trao đổi giữa người dùng
- Thương lượng và xác nhận trao đổi
- Quản lý phí vận chuyển

### 🏺 Đấu giá (Auction)
- Tạo yêu cầu đấu giá (seller)
- Phê duyệt yêu cầu (moderator)
- Hệ thống đấu giá real-time với SSE
- Thanh toán và xử lý sau đấu giá

### 💰 Thanh toán & Ví
- Ví điện tử internal
- Tích hợp ZaloPay
- Yêu cầu rút tiền với approval workflow
- Quản lý tài khoản ngân hàng

### 📦 Vận chuyển
- Tích hợp Giao Hàng Nhanh (GHN)
- Tự động tracking đơn hàng
- Tính phí vận chuyển real-time

### 🔔 Thông báo
- Push notifications qua Firebase
- Email notifications
- Discord webhook integration
- Server-Sent Events cho real-time updates

### 📊 Quản trị
- Dashboard cho Admin/Moderator
- Quản lý người dùng và đơn hàng
- Duyệt yêu cầu đấu giá
- Xử lý yêu cầu rút tiền

## API Endpoints

### Authentication
```
POST   /v1/auth/login                 # Đăng nhập
POST   /v1/auth/google-login          # Đăng nhập Google OAuth
POST   /v1/tokens/verify              # Verify JWT token
```

### Users
```
POST   /v1/users                      # Tạo tài khoản
GET    /v1/users/:id                  # Lấy thông tin user
PUT    /v1/users/:id                  # Cập nhật user
PATCH  /v1/users/:id/avatar           # Cập nhật avatar
```

### Gundams
```
GET    /v1/gundams                    # Danh sách Gundam
GET    /v1/gundams/:id                # Chi tiết Gundam
POST   /v1/users/:id/gundams          # Tạo Gundam mới
```

### Exchange Posts
```
GET    /v1/exchange-posts             # Danh sách bài đăng trao đổi
POST   /v1/users/me/exchange-posts    # Tạo bài đăng
POST   /v1/users/me/exchange-offers   # Tạo đề xuất trao đổi
```

### Auctions
```
GET    /v1/auctions                   # Danh sách đấu giá
GET    /v1/auctions/:id               # Chi tiết đấu giá
GET    /v1/auctions/:id/stream        # SSE stream
POST   /v1/users/me/auctions/:id/bids # Đặt giá
```

### Orders
```
POST   /v1/orders                     # Tạo đơn hàng
GET    /v1/orders                     # Danh sách đơn hàng
GET    /v1/orders/:id                 # Chi tiết đơn hàng
```

Xem full API documentation tại `/swagger/` endpoint.

## Setup & Installation

### Prerequisites
- Go 1.24+
- PostgreSQL 14+
- Redis 6+
- Docker & Docker Compose (optional)

### Environment Variables
Sao chép `app.sample.env` thành `app.env` và điền các thông tin cần thiết:

```bash
cp app.sample.env app.env
```

Các biến môi trường quan trọng:
- `DATABASE_URL`: PostgreSQL connection string
- `REDIS_SERVER_ADDRESS`: Redis server address
- `TOKEN_SECRET_KEY`: JWT signing key
- `CLOUDINARY_URL`: Cloudinary credentials
- `GMAIL_SMTP_*`: Email SMTP settings
- `GHN_*`: Giao Hàng Nhanh credentials
- `NGROK_AUTH_TOKEN`: Ngrok tunnel (for webhooks)

### Chạy với Docker Compose

```bash
# Build và start tất cả services
make compose

# Hoặc manual
docker compose up --build -d
```

### Chạy local development

1. **Setup database:**
```bash
# Chạy PostgreSQL và Redis
docker run --name postgres -e POSTGRES_PASSWORD=secret -e POSTGRES_USER=root -e POSTGRES_DB=gundam_platform -p 5432:5432 -d postgres:14-alpine
docker run --name redis -p 6379:6379 -d redis:6-alpine

# Run migrations
make migrate-up
```

2. **Generate SQLC code:**
```bash
make sqlc
```

3. **Generate Swagger docs:**
```bash
make swagger
```

4. **Start server:**
```bash
go run main.go
```

Server sẽ chạy tại `http://localhost:8080`
Swagger UI tại `http://localhost:8080/swagger/`

### Makefile Commands

```bash
make migrate-up      # Run database migrations
make migrate-down    # Rollback migrations
make sqlc            # Generate SQLC code
make swagger         # Generate Swagger documentation
make compose         # Build và run Docker Compose
make dump-db         # Export database data
```

## Database Schema

Hệ thống sử dụng PostgreSQL với các bảng chính:

- **users**: Thông tin người dùng
- **gundams**: Sản phẩm Gundam
- **orders**: Đơn hàng
- **auctions**: Phiên đấu giá
- **exchange_posts**: Bài đăng trao đổi
- **wallets**: Ví điện tử
- **payments**: Giao dịch thanh toán

Xem chi tiết schema tại `internal/db/migrations/`

## Background Jobs

Hệ thống sử dụng Asynq để xử lý các background jobs:

- **Auction Management**: Start/end auctions, payment reminders
- **Order Tracking**: Auto-update order status từ GHN
- **Notifications**: Send push notifications, emails
- **Scheduled Tasks**: Daily cleanups, statistics

## Monitoring & Logging

- **Zerolog**: Structured logging
- **Health Checks**: Database, Redis connectivity
- **Metrics**: Basic performance metrics
- **Error Tracking**: Centralized error handling

## Deployment

### Production với Fly.io

```bash
# Deploy to Fly.io
fly deploy
```

### Docker Production

```bash
# Build production image
docker build -t gundam-api .

# Run with production config
docker run -p 8080:8080 --env-file .env.prod gundam-api
```

## API Documentation

Swagger documentation có sẵn tại `/swagger/` endpoint khi chạy server.

**Base URL**: `https://gundam-platform-api.fly.dev`
**Version**: v1.0.0

## Contributing

1. Fork project
2. Tạo feature branch (`git checkout -b feature/amazing-feature`)
3. Commit changes (`git commit -m 'Add amazing feature'`)
4. Push to branch (`git push origin feature/amazing-feature`)
5. Tạo Pull Request

## Security

- JWT tokens với expiration
- Password hashing với bcrypt
- Input validation và sanitization
- Rate limiting (planned)
- HTTPS enforce trong production

## License

Dự án này thuộc về team phát triển Gundam Platform.

## Support

Để được hỗ trợ, vui lòng tạo issue trên GitHub repository.