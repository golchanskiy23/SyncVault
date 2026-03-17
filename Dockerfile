FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG SERVICE_NAME
RUN go build -o /bin/service ./cmd/${SERVICE_NAME}/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /bin/service /bin/service
ENTRYPOINT ["/bin/service"]
