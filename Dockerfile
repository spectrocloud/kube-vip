# syntax=docker/dockerfile:experimental

ARG GOLANG_VERSION

FROM --platform=$TARGETPLATFORM gcr.io/spectro-images-public/golang:${GOLANG_VERSION}-alpine as builder
ARG TARGETOS
ARG TARGETARCH
ARG CRYPTO_LIB
ENV GOEXPERIMENT=${CRYPTO_LIB:+boringcrypto}

RUN apk add --no-cache git ca-certificates make
RUN adduser -D appuser
COPY . /src/
WORKDIR /src

ENV GO111MODULE=on
RUN if [ ${CRYPTO_LIB} ]; \
    then \
      go-build-fips.sh -a -o kube-vip ;\
    else \
      go-build-static.sh -a -o kube-vip ;\
    fi

FROM scratch
# Add Certificates into the image, for anything that does API calls
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# Add kube-vip binary
COPY --from=builder /src/kube-vip /
ENTRYPOINT ["/kube-vip"]
