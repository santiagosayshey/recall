# One static binary in a distroless image. The version is baked in at build
# time by the release workflow.
FROM golang:1.26 AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /recall ./cmd/recall

FROM gcr.io/distroless/static:nonroot
COPY --from=build /recall /recall
ENTRYPOINT ["/recall"]
CMD ["serve"]
