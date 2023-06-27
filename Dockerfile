# syntax=docker/dockerfile:experimental

FROM golang:1.19.10-alpine3.18 as dev
# FIPS
ARG CRYPTO_LIB
ENV GOEXPERIMENT=${CRYPTO_LIB:+boringcrypto}

RUN apk add --no-cache git ca-certificates make  gcc g++
RUN adduser -D appuser
COPY . /src/
WORKDIR /src

ENV GO111MODULE=on
RUN --mount=type=cache,sharing=locked,id=gomod,target=/go/pkg/mod/cache \
    --mount=type=cache,sharing=locked,id=goroot,target=/root/.cache/go-build \
    if [ ${CRYPTO_LIB} ]; \
    then \
    CGO_ENABLED=1 FIPS_ENABLE=yes GOOS=linux make build ;\
    else \
    CGO_ENABLED=0 GOOS=linux make build ;\
    fi

FROM scratch
# Add Certificates into the image, for anything that does API calls
COPY --from=dev /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# Add kube-vip binary
COPY --from=dev /src/kube-vip /
ENTRYPOINT ["/kube-vip"]
