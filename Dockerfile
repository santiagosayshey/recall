# One static binary in a distroless image. The version is baked in at build
# time by the release workflow.
FROM golang:1.26 AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /recall ./cmd/recall
RUN mkdir /data

FROM gcr.io/distroless/static:nonroot
COPY --from=build /recall /recall
# An empty /data owned by the runtime user, so a fresh named volume mounted
# there is writable. A bind mount needs the right owner on the host.
COPY --from=build --chown=nonroot:nonroot /data /data
VOLUME /data
ENTRYPOINT ["/recall"]
CMD ["serve"]
