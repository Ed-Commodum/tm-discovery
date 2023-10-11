FROM golang:1.21.3 AS build-stage
WORKDIR /app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code.
COPY *.go ./

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /tm-discovery

FROM gcr.io/distroless/base-debian11 AS release-stage

WORKDIR /

COPY --from=build-stage /tm-discovery /tm-discovery

USER nonroot:nonroot

#Run
ENTRYPOINT ["/tm-discovery"]
