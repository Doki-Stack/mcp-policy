FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/mcp-policy ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/mcp-policy /mcp-policy
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/mcp-policy"]
