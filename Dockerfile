# syntax=docker/dockerfile:experimental

FROM golang:1.20.2-alpine3.17 as dev
RUN apk add --no-cache git ca-certificates make
RUN adduser -D appuser
COPY . /src/
WORKDIR /src

ENV GO111MODULE=on
RUN --mount=type=cache,sharing=locked,id=gomod,target=/go/pkg/mod/cache \
    --mount=type=cache,sharing=locked,id=goroot,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux make build

FROM gcr.io/distroless/static:nonroot
#RUN addgroup -S spectro
#RUN adduser -S -D -h / spectro spectro
#USER spectro
# Add Certificates into the image, for anything that does API calls
COPY --chown=spectro:spectro --from=dev /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# Add kube-vip binary
COPY --chown=spectro:spectro --from=dev /src/kube-vip /
ENTRYPOINT ["/kube-vip"]