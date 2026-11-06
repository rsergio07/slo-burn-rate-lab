FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /shop .

FROM scratch
COPY --from=build /shop /shop
USER 65532
ENTRYPOINT ["/shop"]
