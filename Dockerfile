FROM golang:1.25 AS build

WORKDIR /app
COPY . .

RUN go mod tidy
RUN env CGO_ENABLED=0 go build -o retreat .\cmd\api-server\main.go

FROM alpine:3.20

WORKDIR /app
COPY --from=build /app/retreat .

EXPOSE 8000
CMD ["./retreat", "serve"]
