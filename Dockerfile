FROM golang:1.21-alpine as builder
WORKDIR /workspace

# cache dependencies
COPY app/go.mod app/go.sum ./
RUN go mod download && go mod verify

COPY app .
RUN go build -o /workspace/pgkube .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /workspace/pgkube /pgkube
COPY app/migrations /migrations
COPY app/assets /assets
COPY app/templates /templates
WORKDIR /
USER nonroot:nonroot

ENTRYPOINT ["/pgkube"]
