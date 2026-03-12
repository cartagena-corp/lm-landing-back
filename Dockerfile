# Etapa 1: Compilar el binario
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copiar dependencias primero (mejor cache de Docker)
COPY go.mod go.sum ./
RUN go mod download

# Copiar código fuente y compilar
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /lm-landing-back .

# Etapa 2: Imagen final ultra liviana
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copiar solo el binario compilado
COPY --from=builder /lm-landing-back  .

# Crear directorio para el token
RUN mkdir -p /app/data

EXPOSE 8080

CMD ["./lm-landing-back "]
