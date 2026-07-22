FROM golang:1.26-alpine AS builder
RUN apk --no-cache add gcc musl-dev libwebp-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o mailblogger .

FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata libwebp
WORKDIR /app
COPY --from=builder /app/mailblogger .
COPY --from=builder /app/static ./static
COPY --from=builder /app/themes ./default-theme
COPY --from=builder /app/web/templates ./web/templates
COPY --from=builder /app/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 8080
VOLUME ["/app/content", "/app/config.yaml", "/app/themes"]
ENTRYPOINT ["/entrypoint.sh"]
CMD ["./mailblogger", "serve", "-config", "/app/config.yaml"]
