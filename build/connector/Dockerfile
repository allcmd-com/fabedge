FROM golang:1.16.4 as builder
COPY . /fabedge
RUN cd /fabedge && make connector CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct

FROM alpine:3.11
RUN apk --update add iptables && \
    rm -rf /var/cache/apk/*

COPY --from=builder /fabedge/_output/fabedge-connector /usr/local/bin/connector

CMD ["/usr/local/bin/connector"]