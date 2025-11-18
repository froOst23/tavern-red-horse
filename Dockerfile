FROM golang:1.24-alpine AS builder
RUN apk --no-cache add tzdata
WORKDIR /build
COPY go.mod .
RUN go mod tidy \
    && go mod download
COPY . .
RUN sh -c 'CGO_ENABLED=0 GOOS=linux go build -o app ./cmd/*'

FROM scratch AS final
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /build/app /bin/app/app
COPY --from=builder /build/VERSION /bin/app/VERSION

ENV TZ=Europe/Moscow
EXPOSE 5000
ENTRYPOINT ["/bin/app/app"]
CMD ["-config", "/opt/conf/application.yaml"]
