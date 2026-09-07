FROM golang:1.25-alpine AS build

WORKDIR /src/go-api

COPY go-api/go.mod go-api/go.sum ./
RUN go mod download

COPY go-api/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/flowmind-api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/flowmind-api /flowmind-api
USER nonroot:nonroot
ENTRYPOINT ["/flowmind-api"]
