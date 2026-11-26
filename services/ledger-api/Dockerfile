# Build context is services/ledger-api.
FROM golang:1.27.4 AS build
WORKDIR /src
# Dependencies first: this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ledger-api ./cmd/ledger-api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ledger-api /ledger-api
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/ledger-api"]
