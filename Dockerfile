FROM golang:1.13.4-alpine3.10 AS builder

RUN mkdir /app 
ADD . /app/ 
WORKDIR /app
RUN CGO_ENABLED=0 GOOS=linux go build -mod vendor -a -installsuffix cgo -o server .

FROM alpine:3.11
ARG BRANCH
ENV BRANCH $BRANCH

WORKDIR /

COPY --from=builder /app/server /server
ENTRYPOINT ["./server"]
