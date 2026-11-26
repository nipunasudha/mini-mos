FROM golang:1.27.4 AS build
WORKDIR /src
# Dependencies first, so source edits don't invalidate the download layer.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bill-scheduler ./cmd/bill-scheduler

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bill-scheduler /bill-scheduler
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/bill-scheduler"]
