# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN make build

# Final stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/bin/uptimy-agent /usr/local/bin/uptimy-agent
COPY --from=builder /app/configs/default.yaml /etc/uptimy/config.yaml

USER nonroot:nonroot

ENTRYPOINT ["uptimy-agent"]
CMD ["run", "--config", "/etc/uptimy/config.yaml"]
