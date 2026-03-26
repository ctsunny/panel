# Stage 1: Build frontend
# 始终在 build 机器的原生架构上运行，前端产物与 CPU 架构无关，彻底消除 QEMU 开销
FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend
WORKDIR /app
COPY vite-frontend/package*.json ./
RUN npm install
COPY vite-frontend/ .
RUN npm run build

# Stage 2: Build Go backend
# 在原生架构上通过 Go 内置交叉编译生成目标架构二进制，无需 QEMU
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS backend
ARG TARGETARCH=amd64
WORKDIR /app
COPY go-backend/go.mod go-backend/go.sum ./
RUN go mod download
COPY go-backend/ .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o panel .

# Stage 3: Final minimal image
FROM alpine:3.20
RUN apk --no-cache add tzdata ca-certificates
WORKDIR /app

# Copy Go binary
COPY --from=backend /app/panel ./panel

# Copy frontend static files
COPY --from=frontend /app/dist ./static

# Data directory for SQLite database
VOLUME ["/data"]

EXPOSE 6365

ENV PORT=6365
ENV DB_PATH=/data/gost.db
ENV STATIC_DIR=/app/static
ENV JWT_SECRET=""
ENV LOG_DIR=/data/logs

CMD ["./panel"]
