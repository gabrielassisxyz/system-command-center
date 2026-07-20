# syntax=docker/dockerfile:1

FROM golang:1.26.5-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/hardware-usage ./cmd/hardware-usage

FROM scratch

USER 0:0
COPY --from=build /out/hardware-usage /hardware-usage

ENTRYPOINT ["/hardware-usage"]
