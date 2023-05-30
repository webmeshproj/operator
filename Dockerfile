# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
USER 65532:65532

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ADD dist/operator_${TARGETOS}_${TARGETARCH} /operator

ENTRYPOINT ["/operator"]
