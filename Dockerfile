# Built by GoReleaser (dockers_v2): the build context holds the prebuilt
# binary per platform at $TARGETPLATFORM/envguard. Not meant for `docker build .`.
FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/envguard /usr/local/bin/envguard
USER nonroot:nonroot
WORKDIR /repo
ENTRYPOINT ["/usr/local/bin/envguard"]
CMD ["check"]
