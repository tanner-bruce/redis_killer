FROM golang:1.10 AS bldr

WORKDIR /go/src/github.com/tanner-bruce/redis-killer

COPY main.go ./
COPY cmd cmd/
COPY vendor vendor/

RUN GOOS=linux CGO_ENABLED=false go build main.go && cp main /redis-killer

FROM scratch
COPY --from=bldr /redis-killer /redis-killer
ENTRYPOINT ["/redis-killer"]
