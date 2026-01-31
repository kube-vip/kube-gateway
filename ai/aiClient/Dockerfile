# syntax=docker/dockerfile:experimental

FROM golang:1.25-alpine as dev
RUN apk add --no-cache ca-certificates
COPY . .
ENV GO111MODULE=on
RUN --mount=type=cache,sharing=locked,id=gomod,target=/go/pkg/mod/cache \
    --mount=type=cache,sharing=locked,id=goroot,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /aiclient
 
#### Un-comment when moving to production    
#FROM scratch
# Add Certificates into the image, for anything that does API calls
#COPY --from=dev /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
#COPY --from=dev /aiclient /

ENTRYPOINT /aiclient