# Dockerfile
FROM golang:1.17.1-alpine3.13 as builder
WORKDIR /app
COPY main.go .
RUN go build -o kubevela-demo-cicd-app main.go

FROM alpine:3.10
WORKDIR /app
COPY --from=builder /app/kubevela-demo-cicd-app /app/kubevela-demo-cicd-app
ENTRYPOINT ./kubevela-demo-cicd-app
EXPOSE 8088