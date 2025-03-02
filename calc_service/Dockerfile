FROM golang:1.24-alpine as builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .


RUN go build -o calc-service


FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/calc-service .

EXPOSE 50051

CMD ["./calc-service"]