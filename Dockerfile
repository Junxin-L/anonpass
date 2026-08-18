FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/anonpassd ./cmd/anonpassd
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/anonpassload ./cmd/anonpassload
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/anonpassmigrate ./cmd/anonpassmigrate

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/anonpassd /app/anonpassd
COPY --from=build /out/anonpassload /app/anonpassload
COPY --from=build /out/anonpassmigrate /app/anonpassmigrate
COPY --from=build /src/migrations /app/migrations

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/anonpassd"]
