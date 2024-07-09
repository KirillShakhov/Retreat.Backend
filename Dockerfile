FROM golang:1.22 AS build

WORKDIR /app
COPY . .

RUN go mod tidy
RUN env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o retreat

FROM alpine:3.20

WORKDIR /app
COPY --from=build /app/retreat .

EXPOSE 8000
CMD ["./retreat", "serve"]
