FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY server .
USER 65532:65532

ENTRYPOINT ["/server"]