FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/yat-dice-backend .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -H app
COPY --from=build /out/yat-dice-backend /usr/local/bin/yat-dice-backend
USER app
EXPOSE 8080
ENTRYPOINT ["yat-dice-backend"]
