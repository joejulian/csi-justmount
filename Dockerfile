FROM golang:1.24-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod edit -dropreplace github.com/kubernetes-csi/csi-test \
    -dropreplace github.com/kubernetes-csi/csi-test/v5 && \
    go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/justmount ./main.go

FROM gcr.io/distroless/static:latest
COPY --from=build /out/justmount /justmount
USER 0:0
ENTRYPOINT ["/justmount"]
