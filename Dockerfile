# syntax=docker/dockerfile:experimental
ARG BUILDER_GOLANG_VERSION
# First stage: build the executable.
FROM --platform=$TARGETPLATFORM gcr.io/spectro-images-public/golang:${BUILDER_GOLANG_VERSION}-alpine as dev
# FIPS
ARG CRYPTO_LIB

RUN apk add --no-cache git ca-certificates make  gcc g++
RUN adduser -D appuser
COPY . /src/
WORKDIR /src

ENV GO111MODULE=on
RUN --mount=type=cache,sharing=locked,id=gomod,target=/go/pkg/mod/cache \
    --mount=type=cache,sharing=locked,id=goroot,target=/root/.cache/go-build \
    if [ ${CRYPTO_LIB} ]; \
    then \
    go-build-fips.sh -a -o kube-vip . ;\
    else \
    go-build-static.sh -a -o kube-vip . ;\
    fi

RUN if [ "${CRYPTO_LIB}" ]; then assert-static.sh kube-vip; fi
RUN if [ "${CRYPTO_LIB}" ]; then assert-fips.sh kube-vip; fi
RUN scan-govulncheck.sh kube-vip

FROM scratch
# Add Certificates into the image, for anything that does API calls
COPY --from=dev /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# Add kube-vip binary
COPY --from=dev /src/kube-vip /
ENTRYPOINT ["/kube-vip"]
