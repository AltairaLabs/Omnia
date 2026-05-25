# Minimal stub function-server for the consolidation E2E test.
# Static binary, no external module deps — see main.go.
FROM golang:1.23-alpine AS build
WORKDIR /app
COPY main.go .
RUN CGO_ENABLED=0 go build -o stub .

FROM scratch
COPY --from=build /app/stub /stub
EXPOSE 8080
ENTRYPOINT ["/stub"]
