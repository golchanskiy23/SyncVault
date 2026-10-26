FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# SERVICE_PATH — пакет для сборки; по умолчанию cmd/<SERVICE_NAME> (tools/migrate — для миграций)
ARG SERVICE_NAME
ARG SERVICE_PATH=cmd/${SERVICE_NAME}
RUN CGO_ENABLED=0 go build -o /bin/service ./${SERVICE_PATH}

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /bin/service /bin/service
ENTRYPOINT ["/bin/service"]
